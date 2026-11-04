package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	authcore "hitkeep/auth"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/mailer"
	"hitkeep/security"
	"hitkeep/server/shared"
	"hitkeep/sso"
	"hitkeep/testutil"
)

const nativeVerifier = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"

func nativeStartFixture(t *testing.T, h *handler) (string, *http.ServeMux) {
	t.Helper()
	h.ctx.Config.PublicURL = "https://example.test/prefix"
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	digest := sha256.Sum256([]byte(nativeVerifier))
	input := api.NativeAuthStart{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: "S256", State: "random-client-state", Intent: "login"}
	body, _ := json.Marshal(input)
	req := httptest.NewRequest("POST", "/api/auth/native/start", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("start: %d %s", rr.Code, rr.Body.String())
	}
	var started api.NativeAuthStarted
	if err := json.Unmarshal(rr.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(started.AuthorizationURL)
	if u.Scheme+"://"+u.Host+u.Path != "https://example.test/prefix/api/auth/native/authorize" {
		t.Fatal(started.AuthorizationURL)
	}
	return u.Query().Get("request"), mux
}

func nativeUserCookie(t *testing.T, h *handler, id uuid.UUID) *http.Cookie {
	t.Helper()
	token, err := authcore.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, id)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: authcore.CookieName, Value: token}
}

func TestNativePasswordPinsSubjectBeforeConsent(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateUser(t.Context(), "native-password@example.test", hash)
	if err != nil {
		t.Fatal(err)
	}
	handle, mux := nativeStartFixture(t, h)
	body, _ := json.Marshal(map[string]any{"email": "native-password@example.test", "password": "password123", "return_url": shared.NativeAuthReturnPath(handle)})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/api/login", strings.NewReader(string(body))))
	if rr.Code != 200 {
		t.Fatalf("login %d %s", rr.Code, rr.Body.String())
	}
	entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
	if !ok || entry.UserID != id {
		t.Fatal("primary password did not bind native subject")
	}
	if h.ctx.AuthState.BindNativeAuth(handle, uuid.New()) {
		t.Fatal("changed subject accepted")
	}
}

func TestNativeConsentExchangeAndReplay(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUser(t.Context(), "native-consent@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	handle, mux := nativeStartFixture(t, h)
	cookie := nativeUserCookie(t, h, id)
	req := httptest.NewRequest("GET", "/api/auth/native/consent?request="+handle, nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("consent %d %s", rr.Code, rr.Body.String())
	}
	var consent api.NativeAuthConsent
	if err := json.Unmarshal(rr.Body.Bytes(), &consent); err != nil {
		t.Fatal(err)
	}
	var browser *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == nativeBrowserCookie {
			browser = c
		}
	}
	if browser == nil {
		t.Fatal("missing browser binding")
	}
	form := url.Values{"request": {handle}, "csrf_token": {consent.CSRFToken}, "decision": {"approve"}, "remember_me": {"false"}}
	approve := func(origin string, extra bool) *httptest.ResponseRecorder {
		f := form.Clone()
		if extra {
			f.Add("decision", "deny")
		}
		req := httptest.NewRequest("POST", "/api/auth/native/authorize", strings.NewReader(f.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.AddCookie(cookie)
		req.AddCookie(browser)
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		return out
	}
	for _, origin := range []string{"", "null", "https://evil.test"} {
		if out := approve(origin, false); out.Code != 403 {
			t.Fatalf("origin %q: %d", origin, out.Code)
		}
	}
	if out := approve("https://example.test", true); out.Code != 400 {
		t.Fatalf("duplicate: %d", out.Code)
	}
	out := approve("https://example.test", false)
	if out.Code != 303 {
		t.Fatalf("approval %d %s", out.Code, out.Body.String())
	}
	cb, _ := url.Parse(out.Header().Get("Location"))
	if cb.Query().Get("iss") != "https://example.test/prefix" || cb.Query().Get("state") != "random-client-state" || len(cb.Query()) != 3 {
		t.Fatal("callback bindings")
	}
	body, _ := json.Marshal(api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, Code: cb.Query().Get("code"), CodeVerifier: nativeVerifier})
	for _, want := range []int{200, 400} {
		req := httptest.NewRequest("POST", "/api/auth/native/exchange", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		if out.Code != want {
			t.Fatalf("exchange %d want %d: %s", out.Code, want, out.Body.String())
		}
		if want == 200 {
			found := false
			for _, c := range out.Result().Cookies() {
				if c.Name == authcore.CookieName && c.Value != "" && c.MaxAge >= 0 {
					found = true
				}
			}
			if !found {
				t.Fatal("missing ordinary session")
			}
		}
		if out.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable")
		}
	}
}

func TestNativeStrictInputsOriginsAndBearer(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	_, mux := nativeStartFixture(t, h)
	for _, tc := range []struct {
		body, content, origin, bearer string
		want                          int
	}{
		{`{"client_id":"a","client_id":"b"}`, "application/json", "", "", 400},
		{`{"surprise":true}`, "application/json", "", "", 400},
		{`{}`, "application/x-www-form-urlencoded", "", "", 400},
		{`{}`, "application/json", "null", "", 400},
		{`{}`, "application/json", "https://evil.test", "", 400},
		{`{}`, "application/json", "", "Bearer hk_oauth_at_fake", 403},
	} {
		req := httptest.NewRequest("POST", "/api/auth/native/start", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.content)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if tc.bearer != "" {
			req.Header.Set("Authorization", tc.bearer)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%+v: %d", tc, rr.Code)
		}
	}
}

func TestNativeEmailMFAInAnotherBrowserRequiresFreshConsent(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUser(t.Context(), "native-email@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnableUserTOTP(t.Context(), id, "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatal(err)
	}
	driver := &authTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(driver, h.ctx.Config)
	handle, mux := nativeStartFixture(t, h)
	response, err := h.beginUserLogin(withNativeReturnPath(httptest.NewRequest("POST", "/api/login", nil), shared.NativeAuthReturnPath(handle)), httptest.NewRecorder(), id, false, "")
	if err != nil || response.Status != "mfa_required" {
		t.Fatalf("MFA %+v %v", response, err)
	}
	body, _ := json.Marshal(mfaEmailLinkRequest{ChallengeToken: response.ChallengeToken, ReturnURL: "/dashboard"})
	out := httptest.NewRecorder()
	h.handleMFAEmailLinkRequest()(out, httptest.NewRequest("POST", "/api/auth/mfa/email-link/request", strings.NewReader(string(body))))
	if out.Code != 200 {
		t.Fatalf("mail %d %s", out.Code, out.Body.String())
	}
	token := extractMagicLinkToken(t, driver.textBody)
	verified := httptest.NewRecorder()
	h.handleMFAEmailLinkVerify()(verified, httptest.NewRequest("GET", "/api/auth/mfa/email-link/verify?token="+token, nil))
	if verified.Code != 303 || verified.Header().Get("Location") != "https://example.test/prefix"+shared.NativeAuthReturnPath(handle) {
		t.Fatalf("email return %d %s", verified.Code, verified.Header().Get("Location"))
	}
	entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
	if !ok || entry.Approved {
		t.Fatal("email approved native request")
	}
	req := httptest.NewRequest("GET", "/api/auth/native/consent?request="+handle, nil)
	for _, cookie := range verified.Result().Cookies() {
		req.AddCookie(cookie)
	}
	consent := httptest.NewRecorder()
	mux.ServeHTTP(consent, req)
	if consent.Code != 200 || !strings.Contains(consent.Body.String(), "csrf_token") {
		t.Fatalf("fresh browser consent %d %s", consent.Code, consent.Body.String())
	}
	other, err := store.CreateUser(t.Context(), "native-wrong@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	wrong := httptest.NewRequest("GET", "/api/auth/native/consent?request="+handle, nil)
	wrong.AddCookie(nativeUserCookie(t, h, other))
	denied := httptest.NewRecorder()
	mux.ServeHTTP(denied, wrong)
	if denied.Code != 400 {
		t.Fatalf("wrong final account: %d", denied.Code)
	}
}

func TestNativeSocialCompletionsPreserveAllProviderSubjects(t *testing.T) {
	for _, provider := range []string{"google", "github", "microsoft"} {
		t.Run(provider, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			id, err := store.CreateUser(t.Context(), "native-"+provider+"@example.test", "unused")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.LinkSocialIdentity(t.Context(), database.LinkSocialIdentityInput{UserID: id, Provider: provider, Subject: "native-subject", ObservedEmail: "native-" + provider + "@example.test"}); err != nil {
				t.Fatal(err)
			}
			handle, _ := nativeStartFixture(t, h)
			token := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{Provider: provider, Subject: "native-subject", ObservedEmail: "native-" + provider + "@example.test", EmailVerified: true, Flow: "login", ReturnPath: shared.NativeAuthReturnPath(handle), ExpiresAt: time.Now().Add(time.Minute)})
			body, _ := json.Marshal(map[string]string{"completion_token": token})
			rr := httptest.NewRecorder()
			h.handleSocialComplete()(rr, httptest.NewRequest("POST", "/api/auth/social/complete", strings.NewReader(string(body))))
			if rr.Code != 200 {
				t.Fatalf("social %d %s", rr.Code, rr.Body.String())
			}
			entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
			if !ok || entry.UserID != id || entry.Approved {
				t.Fatal("subject or approval binding")
			}
			if !strings.Contains(rr.Body.String(), shared.NativeAuthReturnPath(handle)) {
				t.Fatal("return path lost")
			}
		})
	}
}

func TestNativeSSOVerifiedProviderContinuation(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "native-sso@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	team := configureTestSSO(t, h, store, provider.issuerURL())
	id := addTestSSOMember(t, store, team, "native-sso@example.com")
	handle, _ := nativeStartFixture(t, h)
	authorization := startTestSSOLogin(t, h, "native-sso@example.com", shared.NativeAuthReturnPath(handle), false)
	u, _ := url.Parse(authorization)
	provider.setNonce(u.Query().Get("nonce"))
	rr := httptest.NewRecorder()
	h.handleSSOCallback()(rr, httptest.NewRequest("GET", "/api/auth/sso/callback?state="+url.QueryEscape(u.Query().Get("state"))+"&code=test-code", nil))
	if rr.Code != 303 || rr.Header().Get("Location") != "https://example.test/prefix"+shared.NativeAuthReturnPath(handle) {
		t.Fatalf("SSO return %d %s", rr.Code, rr.Header().Get("Location"))
	}
	entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
	if !ok || entry.UserID != id || entry.Approved {
		t.Fatal("SSO subject or approval")
	}
}

func TestNativeLocalMFAFactorsRetainSubject(t *testing.T) {
	for _, factor := range []string{"totp", "recovery_code", "passkey"} {
		t.Run(factor, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			id, err := store.CreateUser(t.Context(), "native-factor-"+factor+"@example.test", "unused")
			if err != nil {
				t.Fatal(err)
			}
			const secret = "JBSWY3DPEHPK3PXP"
			if err := store.EnableUserTOTP(t.Context(), id, secret); err != nil {
				t.Fatal(err)
			}
			fixture, err := testutil.NewPasskeyFixture()
			if err != nil {
				t.Fatal(err)
			}
			if factor == "passkey" {
				if _, err := store.CreateUserPasskeyCredential(t.Context(), id, "Native factor", fixture.Credential()); err != nil {
					t.Fatal(err)
				}
			}
			code, err := security.GenerateCurrentTOTPCode(secret, time.Now())
			if err != nil {
				t.Fatal(err)
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
			handle, _ := nativeStartFixture(t, h)
			response, err := h.beginUserLogin(withNativeReturnPath(httptest.NewRequest("POST", "/api/login", nil), shared.NativeAuthReturnPath(handle)), httptest.NewRecorder(), id, false, "")
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(mfaTotpVerifyRequest{ChallengeToken: response.ChallengeToken, Code: code})
			rr := httptest.NewRecorder()
			switch factor {
			case "totp":
				h.handleMFATOTPVerify()(rr, httptest.NewRequest("POST", "/api/auth/mfa/totp/verify", strings.NewReader(string(body))))
			case "recovery_code":
				h.handleMFARecoveryCodeVerify()(rr, httptest.NewRequest("POST", "/api/auth/mfa/recovery-code/verify", strings.NewReader(string(body))))
			case "passkey":
				credential, err := fixture.AssertionResponse(response.Passkey.Challenge, "https://example.test", "example.test", id[:], 1, true)
				if err != nil {
					t.Fatal(err)
				}
				body, _ = json.Marshal(map[string]any{"challenge_token": response.ChallengeToken, "credential": credential})
				h.handlePasskeyLoginFinish()(rr, httptest.NewRequest("POST", "/api/auth/passkey/login/finish", strings.NewReader(string(body))))
			}
			if rr.Code != 200 {
				t.Fatalf("factor %d %s", rr.Code, rr.Body.String())
			}
			entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
			if !ok || entry.UserID != id || entry.Approved {
				t.Fatal("MFA lost binding or approved native")
			}
		})
	}
}

func TestNativeRememberChoiceAndSecurityRecheck(t *testing.T) {
	for _, mode := range []string{"remember", "session", "password-changed", "social-linked", "deleted", "logout"} {
		t.Run(mode, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "native-security-"+mode+"@example.test", "unused")
			if err != nil {
				t.Fatal(err)
			}
			handle, mux := nativeStartFixture(t, h)
			req := httptest.NewRequest("GET", "/api/auth/native/consent", nil)
			_, fingerprint, err := h.nativeAccountSecurity(req, id)
			if err != nil {
				t.Fatal(err)
			}
			csrf, _, ok := h.ctx.AuthState.RenderNativeAuth(handle, "browser", id, fingerprint)
			if !ok {
				t.Fatal("render")
			}
			remember := mode == "remember"
			code, ok := h.ctx.AuthState.DecideNativeAuth(handle, "browser", csrf, id, fingerprint, true, remember)
			if !ok {
				t.Fatal("approve")
			}
			want := 200
			switch mode {
			case "password-changed":
				if err := store.UpdatePasswordByID(t.Context(), id.String(), "changed"); err != nil {
					t.Fatal(err)
				}
				want = 400
			case "social-linked":
				if _, err := store.LinkSocialIdentity(t.Context(), database.LinkSocialIdentityInput{UserID: id, Provider: "github", Subject: "new-security-factor"}); err != nil {
					t.Fatal(err)
				}
				want = 400
			case "deleted":
				if _, err := store.DB().ExecContext(t.Context(), "DELETE FROM users WHERE id = ?", id); err != nil {
					t.Fatal(err)
				}
				want = 400
			case "logout":
				h.ctx.AuthState.InvalidateNativeAuth(id)
				want = 400
			}
			input := api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, Code: code, CodeVerifier: nativeVerifier}
			body, _ := json.Marshal(input)
			request := httptest.NewRequest("POST", "/api/auth/native/exchange", strings.NewReader(string(body)))
			request.Header.Set("Content-Type", "application/json")
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, request)
			if out.Code != want {
				t.Fatalf("exchange %d %s", out.Code, out.Body.String())
			}
			for _, cookie := range out.Result().Cookies() {
				if cookie.Name == authcore.RememberMeCookieName && cookie.Value != "" {
					if !remember {
						t.Fatal("unapproved remember")
					}
					actual, err := store.ValidateRememberMeToken(t.Context(), cookie.Value)
					if err != nil || actual != id {
						t.Fatal("remember subject")
					}
				}
			}
		})
	}
}

func TestSessionExtendRechecksCurrentAccount(t *testing.T) {
	for _, mode := range []string{"deleted", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			id := uuid.New()
			want := 401
			if mode == "unavailable" {
				h.ctx.Store = nil
				want = 503
			}
			req := httptest.NewRequest("POST", "/api/auth/session/extend", nil)
			req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, id))
			rr := httptest.NewRecorder()
			h.handleExtendSession()(rr, req)
			if rr.Code != want {
				t.Fatalf("extend %d want %d", rr.Code, want)
			}
			for _, cookie := range rr.Result().Cookies() {
				if cookie.Value != "" {
					t.Fatal("issued cookie without live current account")
				}
			}
		})
	}
}

func TestNativeMicrosoftConfirmationInAnotherBrowser(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUser(t.Context(), "native-ms-confirm@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	handle, _ := nativeStartFixture(t, h)
	token, err := store.CreatePendingSocialConfirmation(t.Context(), database.PendingSocialConfirmation{Provider: "microsoft", Subject: "native-confirmed-subject", ObservedEmail: "native-ms-confirm@example.test", TargetEmail: "native-ms-confirm@example.test", TargetUserID: &id, ReturnPath: shared.NativeAuthReturnPath(handle)})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.handleSocialConfirmation()(rr, httptest.NewRequest("GET", "/api/auth/social/confirm?token="+url.QueryEscape(token), nil))
	if rr.Code != 303 {
		t.Fatalf("confirmation %d %s", rr.Code, rr.Body.String())
	}
	location, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Query().Get("returnUrl") != shared.NativeAuthReturnPath(handle) {
		t.Fatal("confirmation lost native ref")
	}
	fragment, _ := url.ParseQuery(location.Fragment)
	body, _ := json.Marshal(map[string]string{"completion_token": fragment.Get("social_token")})
	completed := httptest.NewRecorder()
	h.handleSocialComplete()(completed, httptest.NewRequest("POST", "/api/auth/social/complete", strings.NewReader(string(body))))
	if completed.Code != 200 {
		t.Fatalf("completion %d %s", completed.Code, completed.Body.String())
	}
	entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
	if !ok || entry.UserID != id || entry.Approved {
		t.Fatal("email confirmation must bind account but never approve native")
	}
}

func TestNativeExchangeDatabaseFailureCannotIssueCookies(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "native-db-failure@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	handle, mux := nativeStartFixture(t, h)
	req := httptest.NewRequest("GET", "/api/auth/native/consent", nil)
	_, fingerprint, err := h.nativeAccountSecurity(req, id)
	if err != nil {
		t.Fatal(err)
	}
	csrf, _, _ := h.ctx.AuthState.RenderNativeAuth(handle, "browser", id, fingerprint)
	code, _ := h.ctx.AuthState.DecideNativeAuth(handle, "browser", csrf, id, fingerprint, true, false)
	if err := store.DB().Close(); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, Code: code, CodeVerifier: nativeVerifier})
	request := httptest.NewRequest("POST", "/api/auth/native/exchange", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, request)
	if rr.Code != 503 {
		t.Fatalf("database failure %d %s", rr.Code, rr.Body.String())
	}
	if len(rr.Result().Cookies()) != 0 {
		t.Fatal("database failure issued cookies")
	}
}

func TestNativeRememberIssuanceFailureConsumesCode(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "native-issuance-failure@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	handle, mux := nativeStartFixture(t, h)
	_, fingerprint, err := h.nativeAccountSecurity(httptest.NewRequest("GET", "/", nil), id)
	if err != nil {
		t.Fatal(err)
	}
	csrf, _, _ := h.ctx.AuthState.RenderNativeAuth(handle, "browser", id, fingerprint)
	code, _ := h.ctx.AuthState.DecideNativeAuth(handle, "browser", csrf, id, fingerprint, true, true)
	if _, err := store.DB().ExecContext(t.Context(), "DROP TABLE remember_me_tokens"); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, Code: code, CodeVerifier: nativeVerifier})
	for _, want := range []int{503, 400} {
		req := httptest.NewRequest("POST", "/api/auth/native/exchange", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != want || len(rr.Result().Cookies()) != 0 {
			t.Fatalf("failure/replay status=%d cookies=%d want=%d", rr.Code, len(rr.Result().Cookies()), want)
		}
	}
}

func TestNativeCanonicalHTTPSIssuesOnlySecureCookies(t *testing.T) {
	for _, remember := range []bool{false, true} {
		t.Run(fmt.Sprint(remember), func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "native-secure-cookie@example.test", "unused")
			if err != nil {
				t.Fatal(err)
			}
			handle, mux := nativeStartFixture(t, h)
			h.ctx.Config.PublicURL = "HTTPS://EXAMPLE.test:443/prefix"
			if !h.ctx.NativeAuthReady() {
				t.Fatal("canonical HTTPS must be supported")
			}
			_, fingerprint, err := h.nativeAccountSecurity(httptest.NewRequest("GET", "/", nil), id)
			if err != nil {
				t.Fatal(err)
			}
			csrf, _, _ := h.ctx.AuthState.RenderNativeAuth(handle, "browser", id, fingerprint)
			code, _ := h.ctx.AuthState.DecideNativeAuth(handle, "browser", csrf, id, fingerprint, true, remember)
			body, _ := json.Marshal(api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, Code: code, CodeVerifier: nativeVerifier})
			request := httptest.NewRequest("POST", "/api/auth/native/exchange", strings.NewReader(string(body)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatalf("exchange %d %s", response.Code, response.Body.String())
			}
			issued := 0
			for _, cookie := range response.Result().Cookies() {
				if cookie.Value != "" {
					issued++
					if !cookie.Secure {
						t.Errorf("canonical HTTPS produced insecure %s", cookie.Name)
					}
					if cookie.Name == authcore.CookieName {
						claims, err := authcore.ValidateTokenClaims(cookie.Value, h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL)
						if err != nil || claims.Issuer != h.ctx.Config.PublicURL {
							t.Fatal("raw JWT issuer changed")
						}
					}
				}
			}
			want := 1
			if remember {
				want = 2
			}
			if issued != want {
				t.Fatalf("issued %d want %d", issued, want)
			}
		})
	}
}
