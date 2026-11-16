package user

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	authcore "hitkeep/auth"
	json "hitkeep/jsonapi"
	"hitkeep/security"
	serverauth "hitkeep/server/auth"
	"hitkeep/server/shared"
	"hitkeep/testutil"
)

func credentialRequest(mux *http.ServeMux, method, path string, body []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, r)
	return out
}

// Both deliveries use the real password and configured-factor endpoints.
func credentialDeletionProof(t *testing.T, h *handler, mux *http.ServeMux, id uuid.UUID, native bool) (string, string, []*http.Cookie) {
	t.Helper()
	hash := strings.Repeat("a", 64)
	attempt := h.ctx.AuthState.CreateAccountDeletionAttempt(id, h.ctx.Config.PublicURL, hash, "/settings", time.Now().Add(5*time.Minute))
	if attempt == "" {
		t.Fatal("attempt unavailable")
	}
	target := shared.AccountDeletionReturnPath(attempt)
	handle := ""
	const verifier = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	if native {
		digest := sha256.Sum256([]byte(verifier))
		body, _ := json.Marshal(api.NativeAuthStart{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: "S256", State: "credential-proof-state", Intent: "login", Purpose: "account_delete", ReauthAttempt: attempt})
		out := credentialRequest(mux, "POST", "/api/auth/native/start", body, nil)
		if out.Code != 200 {
			t.Fatalf("native start %d %s", out.Code, out.Body.String())
		}
		var started api.NativeAuthStarted
		if err := json.Unmarshal(out.Body.Bytes(), &started); err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(started.AuthorizationURL)
		handle = u.Query().Get("request")
		target = shared.NativeAuthReturnPath(handle)
	}
	user, err := h.ctx.Store.GetUserByID(t.Context(), id)
	if err != nil || user == nil {
		t.Fatal("proof user missing")
	}
	body, _ := json.Marshal(map[string]any{"email": user.Email, "password": "password123", "return_url": target})
	out := credentialRequest(mux, "POST", "/api/login", body, nil)
	if out.Code != 200 {
		t.Fatalf("actual primary %d %s", out.Code, out.Body.String())
	}
	var response struct {
		Status    string `json:"status"`
		Challenge string `json:"challenge_token"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status == "mfa_required" {
		secret, found, err := h.ctx.Store.GetUserTOTPSecret(t.Context(), id)
		if err != nil || !found {
			t.Fatal("expected configured TOTP")
		}
		code, err := security.GenerateCurrentTOTPCode(secret, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		body, _ = json.Marshal(map[string]string{"challenge_token": response.Challenge, "code": code})
		out = credentialRequest(mux, "POST", "/api/auth/mfa/totp/verify", body, nil)
		if out.Code != 200 {
			t.Fatalf("actual factor %d %s", out.Code, out.Body.String())
		}
	}
	cookies := out.Result().Cookies()
	if native {
		consent := credentialRequest(mux, "GET", "/api/auth/native/consent?request="+handle, nil, cookies)
		if consent.Code != 200 {
			t.Fatalf("consent %d %s", consent.Code, consent.Body.String())
		}
		var details api.NativeAuthConsent
		if err := json.Unmarshal(consent.Body.Bytes(), &details); err != nil {
			t.Fatal(err)
		}
		cookies = append(cookies, consent.Result().Cookies()...)
		form := url.Values{"request": {handle}, "csrf_token": {details.CSRFToken}, "decision": {"approve"}, "remember_me": {"false"}}
		r := httptest.NewRequest("POST", "/api/auth/native/authorize", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", h.ctx.Config.PublicURL)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		decision := httptest.NewRecorder()
		mux.ServeHTTP(decision, r)
		if decision.Code != 303 {
			t.Fatalf("approval %d %s", decision.Code, decision.Body.String())
		}
		callback, _ := url.Parse(decision.Header().Get("Location"))
		body, _ = json.Marshal(api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, Code: callback.Query().Get("code"), CodeVerifier: verifier})
		out = credentialRequest(mux, "POST", "/api/auth/native/exchange", body, nil)
		if out.Code != 200 {
			t.Fatalf("exchange %d %s", out.Code, out.Body.String())
		}
		cookies = out.Result().Cookies()
	}
	var current []*http.Cookie
	jti := ""
	for _, cookie := range cookies {
		if cookie.Value != "" && cookie.MaxAge >= 0 {
			current = append(current, cookie)
			if cookie.Name == authcore.CookieName {
				claims, err := authcore.ValidateTokenClaims(cookie.Value, h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL)
				if err != nil {
					t.Fatal(err)
				}
				jti = claims.ID
			}
		}
	}
	if !h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, jti, hash) {
		t.Fatal("actual primary/factors did not establish proof")
	}
	return attempt, jti, current
}

func TestCredentialMutationsInvalidateDeletionProofBeforePostRead(t *testing.T) {
	for _, operation := range []string{"email", "enable_totp", "disable_totp", "register_passkey"} {
		for _, postReadFails := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "/browser", true: "/native"}[postReadFails], func(t *testing.T) {
				native := postReadFails
				postReadFails = native && operation != "email"
				h, store, id := setupUserSecurityTestEnv(t)
				defer store.Close()
				h.ctx.Config.PublicURL = "https://example.test"
				if operation == "email" {
					hashed, err := serverauth.HashPassword("password123")
					if err != nil {
						t.Fatal(err)
					}
					id, err = store.CreateUserWithoutDefaultTenant(t.Context(), "profile@example.test", hashed)
					if err != nil {
						t.Fatal(err)
					}
				}
				const secret = "JBSWY3DPEHPK3PXP"
				if operation == "disable_totp" {
					if err := store.EnableUserTOTP(t.Context(), id, secret); err != nil {
						t.Fatal(err)
					}
				}
				remembered, err := store.CreateRememberMeToken(t.Context(), id)
				if err != nil {
					t.Fatal(err)
				}
				mux := http.NewServeMux()
				serverauth.Register(mux, h.ctx)
				Register(mux, h.ctx)
				attempt, jti, cookies := credentialDeletionProof(t, h, mux, id, native)
				method, path := "POST", ""
				var body []byte
				switch operation {
				case "email":
					method = "PUT"
					path = "/api/user/profile"
					body = []byte(`{"email":"changed@example.test"}`)
				case "enable_totp":
					setup := credentialRequest(mux, "POST", "/api/user/security/totp/setup/start", []byte(`{}`), cookies)
					if setup.Code != 200 {
						t.Fatal(setup.Body.String())
					}
					pending, _, found := h.ctx.AuthState.GetPendingTOTPSetup(id)
					if !found {
						t.Fatal("TOTP setup missing")
					}
					code, err := security.GenerateCurrentTOTPCode(pending, time.Now())
					if err != nil {
						t.Fatal(err)
					}
					body, _ = json.Marshal(map[string]string{"code": code})
					path = "/api/user/security/totp/setup/verify"
				case "disable_totp":
					code, err := security.GenerateCurrentTOTPCode(secret, time.Now())
					if err != nil {
						t.Fatal(err)
					}
					body, _ = json.Marshal(map[string]string{"code": code})
					path = "/api/user/security/totp/disable"
				case "register_passkey":
					setup := credentialRequest(mux, "POST", "/api/user/security/passkeys/register/start", []byte(`{"name":"New key"}`), cookies)
					if setup.Code != 200 {
						t.Fatal(setup.Body.String())
					}
					var start passkeyRegistrationStartResponse
					if err := json.Unmarshal(setup.Body.Bytes(), &start); err != nil {
						t.Fatal(err)
					}
					fixture, err := testutil.NewPasskeyFixture()
					if err != nil {
						t.Fatal(err)
					}
					registration, err := fixture.RegistrationResponse(start.PublicKey.Challenge, "https://example.test", "example.test")
					if err != nil {
						t.Fatal(err)
					}
					body, _ = json.Marshal(registration)
					path = "/api/user/security/passkeys/register/finish"
				}
				if postReadFails {
					if err := store.Exec(t.Context(), "DROP TABLE user_recovery_codes"); err != nil {
						t.Fatal(err)
					}
				}
				out := credentialRequest(mux, method, path, body, cookies)
				want := 200
				if postReadFails {
					want = 500
				}
				if out.Code != want {
					t.Fatalf("mutation %d want%d: %s", out.Code, want, out.Body.String())
				}
				switch operation {
				case "email":
					user, err := store.GetUserByID(t.Context(), id)
					if err != nil || user.Email != "changed@example.test" {
						t.Fatal("email did not commit")
					}
				case "enable_totp", "disable_totp":
					enabled, err := store.HasEnabledTOTP(t.Context(), id)
					if err != nil || enabled != (operation == "enable_totp") {
						t.Fatal("TOTP mutation did not commit")
					}
				case "register_passkey":
					keys, err := store.ListUserPasskeys(t.Context(), id)
					if err != nil || len(keys) != 1 {
						t.Fatal("passkey did not commit")
					}
				}
				if h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, jti, strings.Repeat("a", 64)) || h.ctx.AuthState.ConsumeAccountDeletionProof(attempt, id, jti, strings.Repeat("a", 64)) {
					t.Fatal("committed credential mutation retained deletion proof")
				}
				if actual, err := store.ValidateRememberMeToken(t.Context(), remembered); err != nil || actual != id {
					t.Fatal("ordinary remember behavior changed")
				}
				if session := credentialRequest(mux, "GET", "/api/auth/session", nil, cookies); session.Code != 200 {
					t.Fatal("ordinary JWT no longer accepted")
				}
			})
		}
	}
}
