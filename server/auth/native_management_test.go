package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"hitkeep/api"
	authcore "hitkeep/auth"
	json "hitkeep/jsonapi"
	"hitkeep/mailer"
	"hitkeep/security"
	"hitkeep/server/shared"
	"hitkeep/socialauth"
	"hitkeep/sso"
	"hitkeep/testutil"
)

func TestNativeManagementStartRequiresCurrentSessionWithoutIssuance(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "management@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	h.ctx.Config.PublicURL = "https://example.test/prefix"
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	digest := sha256.Sum256([]byte(nativeVerifier))
	body, _ := json.Marshal(map[string]string{"client_id": "hitkeep-ios", "redirect_uri": shared.NativeIOSCallback, "code_challenge": base64.RawURLEncoding.EncodeToString(digest[:]), "code_challenge_method": "S256", "state": "random-client-state", "action": "passkey_register"})
	for _, authenticated := range []bool{false, true} {
		r := httptest.NewRequest("POST", "/api/auth/native/management/start", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		if authenticated {
			r.AddCookie(nativeUserCookie(t, h, id))
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		want := 401
		if authenticated {
			want = 200
		}
		if w.Code != want || len(w.Result().Cookies()) != 0 {
			t.Fatalf("authenticated=%t status=%d cookies=%d body=%s", authenticated, w.Code, len(w.Result().Cookies()), w.Body.String())
		}
		if authenticated {
			var started api.NativeAuthStarted
			if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil || !strings.HasPrefix(started.AuthorizationURL, "https://example.test/prefix/api/auth/native/authorize?request=") {
				t.Fatalf("start %s %v", w.Body.String(), err)
			}
		}
	}
}

func TestNativeManagementRememberOnlyCannotRestoreOrConsume(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "management-remember@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	h.ctx.Config.PublicURL = "https://example.test/prefix"
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	remember, err := store.CreateRememberMeToken(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, authcore.Claims{UserID: id, Issuer: h.ctx.Config.PublicURL, Audience: jwt.ClaimStrings{h.ctx.Config.PublicURL}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}).SignedString([]byte(h.ctx.Config.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	for _, withExpired := range []bool{false, true} {
		for _, path := range []string{"/api/auth/native/management/start", "/api/auth/native/management/complete"} {
			r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(&http.Cookie{Name: authcore.RememberMeCookieName, Value: remember})
			if withExpired {
				r.AddCookie(&http.Cookie{Name: authcore.CookieName, Value: expired})
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 401 || len(w.Result().Cookies()) != 0 {
				t.Fatalf("remember-only status=%d cookies=%d", w.Code, len(w.Result().Cookies()))
			}
		}
	}
}

func TestNativeManagementStrictWireAndAmbientAuthority(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "management-wire@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	h.ctx.Config.PublicURL = "https://example.test/prefix"
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	digest := sha256.Sum256([]byte(nativeVerifier))
	body := `{"client_id":"hitkeep-ios","redirect_uri":"` + shared.NativeIOSCallback + `","code_challenge":"` + base64.RawURLEncoding.EncodeToString(digest[:]) + `","code_challenge_method":"S256","state":"random-client-state","action":"passkey_register"}`
	for _, tc := range []struct {
		name, extra, origin, header string
		duplicate                   bool
		want                        int
	}{
		{name: "native omitted headers", want: 200}, {name: "same origin", origin: "https://example.test", want: 200},
		{name: "foreign origin", origin: "https://foreign.test", want: 400}, {name: "null origin", origin: "null", want: 400}, {name: "duplicate origin", origin: "https://example.test", duplicate: true, want: 400},
		{name: "empty forbidden provider", extra: `,"provider":""`, want: 400}, {name: "null forbidden provider", extra: `,"provider":null`, want: 400}, {name: "forbidden team", extra: `,"team_id":""`, want: 400},
		{name: "actor override", extra: `,"user_id":"00000000-0000-0000-0000-000000000001"`, want: 400}, {name: "purpose override", extra: `,"purpose":"account_delete"`, want: 400}, {name: "remember override", extra: `,"remember_me":false`, want: 400},
		{name: "duplicate action", extra: `,"action":"passkey_register"`, want: 400}, {name: "bearer", header: "Authorization", want: 403}, {name: "api key", header: "X-Api-Key", want: 403}, {name: "agent", header: "X-Hitkeep-Agent", want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := strings.TrimSuffix(body, "}") + tc.extra + "}"
			r := httptest.NewRequest("POST", "/api/auth/native/management/start", strings.NewReader(payload))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(nativeUserCookie(t, h, id))
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.duplicate {
				r.Header.Add("Origin", tc.origin)
			}
			if tc.header != "" {
				r.Header.Set(tc.header, "ambient")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.want || len(w.Result().Cookies()) != 0 {
				t.Fatalf("status=%d want=%d cookies=%d body=%s", w.Code, tc.want, len(w.Result().Cookies()), w.Body.String())
			}
		})
	}
}

func TestNativeManagementSocialUsesBrowserJarAndPersistenceReceipt(t *testing.T) {
	for _, provider := range []string{"google", "github", "microsoft"} {
		t.Run(provider, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "management-social@example.test", "unused")
			if err != nil {
				t.Fatal(err)
			}
			h.ctx.Config.PublicURL = "https://example.test/prefix"
			h.ctx.Config.SocialGoogleClientID = "deletion-client"
			h.ctx.Config.SocialGoogleClientSecret = "fixture-secret"
			h.ctx.Config.SocialGitHubClientID = "deletion-client"
			h.ctx.Config.SocialGitHubClientSecret = "fixture-secret"
			h.ctx.Config.SocialMicrosoftClientID = "deletion-client"
			h.ctx.Config.SocialMicrosoftClientSecret = "fixture-secret"
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}
			fixture := &deletionSocialProvider{name: provider, subject: "management-subject", email: "management-social@example.test", key: key}
			h.ctx.SocialAuth = socialauth.NewClient(sso.NewClient(&http.Client{Transport: fixture}))
			cookie := nativeUserCookie(t, h, id)
			claims, err := authcore.ValidateTokenClaims(cookie.Value, h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL)
			if err != nil {
				t.Fatal(err)
			}
			_, security, err := h.nativeAccountSecurity(httptest.NewRequest("GET", "/", nil), id)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte(nativeVerifier))
			handle, err := h.ctx.AuthState.CreateNativeManagement(api.NativeManagementStart{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), State: "random-client-state"}, id, "https://example.test/prefix", security, shared.NativeManagement{Action: "social_link", Provider: provider})
			if err != nil {
				t.Fatal(err)
			}
			if !h.ctx.AuthState.CompleteAccountDeletionPrimary(shared.NativeManagementReturnPath(handle), id, "https://example.test/prefix", claims.ID) {
				t.Fatal("primary")
			}
			csrf, _, _ := h.ctx.AuthState.RenderNativeAuth(handle, "browser", id, security)
			if _, ok := h.ctx.AuthState.DecideNativeAuth(handle, "browser", csrf, id, security, true, false); !ok {
				t.Fatal("approval")
			}
			mux := http.NewServeMux()
			Register(mux, h.ctx)
			start := httptest.NewRequest("POST", "/api/user/security/social/"+provider+"/start", strings.NewReader(`{}`))
			start.Header.Set("Content-Type", "application/json")
			start.Header.Set("Origin", "https://example.test")
			start.Header.Set(shared.NativeManagementHeader, handle)
			start.AddCookie(cookie)
			start.AddCookie(&http.Cookie{Name: shared.NativeManagementBrowserCookie, Value: "browser"})
			begun := httptest.NewRecorder()
			mux.ServeHTTP(begun, start)
			if begun.Code != 200 {
				t.Fatalf("start=%d %s", begun.Code, begun.Body.String())
			}
			var started struct {
				AuthURL string `json:"auth_url"`
			}
			_ = json.Unmarshal(begun.Body.Bytes(), &started)
			u, _ := url.Parse(started.AuthURL)
			fixture.nonce = u.Query().Get("nonce")
			fixture.challenge = u.Query().Get("code_challenge")
			callbackPath := "/api/auth/social/" + provider + "/callback?state=" + u.Query().Get("state") + "&code=fixture-code"
			callback := func(socialCookies bool, browser string) *httptest.ResponseRecorder {
				r := httptest.NewRequest("GET", callbackPath, nil)
				r.AddCookie(cookie)
				r.AddCookie(&http.Cookie{Name: shared.NativeManagementBrowserCookie, Value: browser})
				if socialCookies {
					for _, c := range begun.Result().Cookies() {
						r.AddCookie(c)
					}
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				return w
			}
			missing := callback(false, "browser")
			if missing.Code != 303 || !strings.Contains(missing.Header().Get("Location"), "social_state_invalid") {
				t.Fatal("provider browser cookie not required")
			}
			linked := callback(true, "browser")
			if linked.Code != 303 || linked.Header().Get("Location") != "https://example.test/prefix"+shared.NativeManagementReturnPath(handle) {
				t.Fatalf("link=%d %s %s", linked.Code, linked.Header().Get("Location"), linked.Body.String())
			}
			identities, err := store.ListUserSocialIdentities(t.Context(), id)
			if err != nil || len(identities) != 1 || identities[0].Provider != provider {
				t.Fatal("provider identity not persisted")
			}
			entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
			if !ok || entry.Management.Phase != "succeeded" {
				t.Fatal("security mutation destroyed own receipt")
			}
			replay := callback(true, "browser")
			if replay.Code != 303 || !strings.Contains(replay.Header().Get("Location"), "social_state_invalid") {
				t.Fatal("provider callback replay")
			}
		})
	}
}

func TestNativeManagementPrimaryCompletesOnlyAfterAllMFA(t *testing.T) {
	for _, factor := range []string{"totp", "recovery_code", "passkey", "email_link"} {
		t.Run(factor, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			h.ctx.Config.PublicURL = "https://example.test/prefix"
			hash, err := HashPassword("password123")
			if err != nil {
				t.Fatal(err)
			}
			id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "management-mfa@example.test", hash)
			if err != nil {
				t.Fatal(err)
			}
			const secret = "JBSWY3DPEHPK3PXP"
			if err := store.EnableUserTOTP(t.Context(), id, secret); err != nil {
				t.Fatal(err)
			}
			code, err := security.GenerateCurrentTOTPCode(secret, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			fixture, err := testutil.NewPasskeyFixture()
			if err != nil {
				t.Fatal(err)
			}
			if factor == "passkey" {
				if _, err := store.CreateUserPasskeyCredential(t.Context(), id, "Existing primary factor", fixture.Credential()); err != nil {
					t.Fatal(err)
				}
			}
			if factor == "recovery_code" {
				codes, err := security.GenerateRecoveryCodes()
				if err != nil {
					t.Fatal(err)
				}
				code = codes[0]
				hash, err := security.HashRecoveryCode(code)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.ReplaceUserRecoveryCodes(t.Context(), id, []string{hash}); err != nil {
					t.Fatal(err)
				}
			}
			driver := &authTestMailDriver{}
			if factor == "email_link" {
				h.ctx.Mailer = mailer.NewWithDriver(driver, h.ctx.Config)
			}
			_, fingerprint, err := h.nativeAccountSecurity(httptest.NewRequest("GET", "/", nil), id)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := h.ctx.AuthState.CreateNativeManagement(api.NativeManagementStart{}, id, "https://example.test/prefix", fingerprint, shared.NativeManagement{Action: "passkey_register"})
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			Register(mux, h.ctx)
			body, _ := json.Marshal(map[string]string{"email": "management-mfa@example.test", "password": "password123", "return_url": shared.NativeManagementReturnPath(handle)})
			logged := httptest.NewRecorder()
			mux.ServeHTTP(logged, deletionJSONRequest("/api/login", body))
			if logged.Code != 200 || len(logged.Result().Cookies()) != 0 {
				t.Fatalf("primary=%d cookies=%d body=%s", logged.Code, len(logged.Result().Cookies()), logged.Body.String())
			}
			var challenge loginResponse
			if err := json.Unmarshal(logged.Body.Bytes(), &challenge); err != nil || challenge.Status != "mfa_required" {
				t.Fatal("primary bypassed MFA")
			}
			entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
			if !ok || entry.PrimaryComplete {
				t.Fatal("MFA pending primary authorized management")
			}
			completed := httptest.NewRecorder()
			switch factor {
			case "totp", "recovery_code":
				body, _ := json.Marshal(mfaTotpVerifyRequest{ChallengeToken: challenge.ChallengeToken, Code: code})
				if factor == "totp" {
					h.handleMFATOTPVerify()(completed, deletionJSONRequest("/api/auth/mfa/totp/verify", body))
				} else {
					h.handleMFARecoveryCodeVerify()(completed, deletionJSONRequest("/api/auth/mfa/recovery-code/verify", body))
				}
			case "passkey":
				credential, err := fixture.AssertionResponse(challenge.Passkey.Challenge, "https://example.test", "example.test", id[:], 1, true)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := json.Marshal(map[string]any{"challenge_token": challenge.ChallengeToken, "credential": credential})
				h.handlePasskeyLoginFinish()(completed, deletionJSONRequest("/api/auth/passkey/login/finish", body))
			case "email_link":
				body, _ := json.Marshal(mfaEmailLinkRequest{ChallengeToken: challenge.ChallengeToken, ReturnURL: "/dashboard"})
				requested := httptest.NewRecorder()
				h.handleMFAEmailLinkRequest()(requested, deletionJSONRequest("/api/auth/mfa/email-link/request", body))
				if requested.Code != 200 {
					t.Fatalf("email request=%d", requested.Code)
				}
				token := extractMagicLinkToken(t, driver.textBody)
				h.handleMFAEmailLinkVerify()(completed, httptest.NewRequest("GET", "/api/auth/mfa/email-link/verify?token="+token, nil))
			}
			if completed.Code != 200 && completed.Code != 303 {
				t.Fatalf("factor=%d %s", completed.Code, completed.Body.String())
			}
			r := httptest.NewRequest("GET", "/api/auth/native/consent?request="+handle, nil)
			for _, cookie := range completed.Result().Cookies() {
				r.AddCookie(cookie)
			}
			consent := httptest.NewRecorder()
			mux.ServeHTTP(consent, r)
			if consent.Code != 200 {
				t.Fatalf("completed MFA consent=%d %s", consent.Code, consent.Body.String())
			}
			entry, ok = h.ctx.AuthState.GetNativeAuth(handle)
			if !ok || !entry.PrimaryComplete || entry.Management.Phase != "pending" || entry.Approved {
				t.Fatal("factor completion must allow explicit consent, never mutation or receipt")
			}
		})
	}
}
