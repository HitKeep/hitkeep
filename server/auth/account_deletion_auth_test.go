package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"maps"
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
	"hitkeep/socialauth"
	"hitkeep/sso"
	"hitkeep/testutil"
)

func deletionAttemptFixture(t *testing.T, h *handler, id uuid.UUID) (string, string) {
	t.Helper()
	h.ctx.Config.PublicURL = "https://example.test/prefix"
	hash := strings.Repeat("a", 64)
	attempt := h.ctx.AuthState.CreateAccountDeletionAttempt(id, shared.NativeAuthIssuer(h.ctx.Config.PublicURL), hash, "/settings", time.Now().Add(5*time.Minute))
	if attempt == "" {
		t.Fatal("create deletion attempt")
	}
	return attempt, hash
}
func responseSessionID(t *testing.T, h *handler, out *httptest.ResponseRecorder) string {
	t.Helper()
	for _, cookie := range out.Result().Cookies() {
		if cookie.Name == authcore.CookieName && cookie.Value != "" {
			claims, err := authcore.ValidateTokenClaims(cookie.Value, h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL)
			if err != nil {
				t.Fatal(err)
			}
			return claims.ID
		}
	}
	t.Fatal("missing session")
	return ""
}
func TestAccountDeletionPasswordRequiresActualPrimaryAndNewJTI(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	password, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateUser(t.Context(), "deletion-password@example.test", password)
	if err != nil {
		t.Fatal(err)
	}
	attempt, hash := deletionAttemptFixture(t, h, id)
	original := httptest.NewRecorder()
	if err := h.issueLoginSession(t.Context(), original, id, false); err != nil {
		t.Fatal(err)
	}
	oldJTI := responseSessionID(t, h, original)
	if h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, oldJTI, hash) {
		t.Fatal("ordinary issuance proved deletion")
	}
	body, _ := json.Marshal(map[string]any{"email": "deletion-password@example.test", "password": "wrong", "return_url": shared.AccountDeletionReturnPath(attempt)})
	failed := httptest.NewRecorder()
	h.handleLogin()(failed, deletionJSONRequest("/api/login", body))
	if failed.Code != 401 {
		t.Fatal(failed.Code)
	}
	body, _ = json.Marshal(map[string]any{"email": "deletion-password@example.test", "password": "password123", "return_url": shared.AccountDeletionReturnPath(attempt)})
	out := httptest.NewRecorder()
	h.handleLogin()(out, deletionJSONRequest("/api/login", body))
	if out.Code != 200 {
		t.Fatalf("login %d %s", out.Code, out.Body.String())
	}
	jti := responseSessionID(t, h, out)
	if jti == oldJTI || !h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, jti, hash) || h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, oldJTI, hash) {
		t.Fatal("fresh proof binding")
	}
	extended := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/auth/session/extend", nil)
	r = r.WithContext(context.WithValue(r.Context(), shared.UserIDKey, id))
	h.handleExtendSession()(extended, r)
	if extended.Code != 200 || h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, responseSessionID(t, h, extended), hash) {
		t.Fatal("extend elevated proof")
	}
}

func TestAccountDeletionNativeForcesPrimaryAndBindsOnlyExchangeJTI(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	password, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateUser(t.Context(), "deletion-native@example.test", password)
	if err != nil {
		t.Fatal(err)
	}
	attempt, hash := deletionAttemptFixture(t, h, id)
	digest := sha256.Sum256([]byte(nativeVerifier))
	input := api.NativeAuthStart{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: "S256", State: "deletion-native-state", Intent: "login", Purpose: "account_delete", ReauthAttempt: attempt}
	body, _ := json.Marshal(input)
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	request := httptest.NewRequest("POST", "/api/auth/native/start", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, request)
	if out.Code != 200 {
		t.Fatalf("start %d %s", out.Code, out.Body.String())
	}
	var started api.NativeAuthStarted
	if err := json.Unmarshal(out.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(started.AuthorizationURL)
	handle := u.Query().Get("request")
	oldCookie := nativeUserCookie(t, h, id)
	authorize := httptest.NewRequest("GET", "/api/auth/native/authorize?request="+handle, nil)
	authorize.AddCookie(oldCookie)
	out = httptest.NewRecorder()
	mux.ServeHTTP(out, authorize)
	if out.Code != 303 || !strings.Contains(out.Header().Get("Location"), "/login?") {
		t.Fatal("current cookie skipped primary")
	}
	consentReq := httptest.NewRequest("GET", "/api/auth/native/consent?request="+handle, nil)
	consentReq.AddCookie(oldCookie)
	out = httptest.NewRecorder()
	mux.ServeHTTP(out, consentReq)
	if out.Code != 400 {
		t.Fatalf("cookie-only consent %d", out.Code)
	}
	body, _ = json.Marshal(map[string]any{"email": "deletion-native@example.test", "password": "password123", "return_url": shared.NativeAuthReturnPath(handle)})
	out = httptest.NewRecorder()
	mux.ServeHTTP(out, deletionJSONRequest("/api/login", body))
	if out.Code != 200 {
		t.Fatalf("primary %d %s", out.Code, out.Body.String())
	}
	browserJTI := responseSessionID(t, h, out)
	if h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, browserJTI, hash) {
		t.Fatal("browser JTI fulfilled native proof")
	}
	var browserCookie *http.Cookie
	for _, c := range out.Result().Cookies() {
		if c.Name == authcore.CookieName {
			browserCookie = c
		}
	}
	consentReq = httptest.NewRequest("GET", "/api/auth/native/consent?request="+handle, nil)
	consentReq.AddCookie(browserCookie)
	out = httptest.NewRecorder()
	mux.ServeHTTP(out, consentReq)
	if out.Code != 200 {
		t.Fatalf("consent %d %s", out.Code, out.Body.String())
	}
	var consent api.NativeAuthConsent
	if err := json.Unmarshal(out.Body.Bytes(), &consent); err != nil {
		t.Fatal(err)
	}
	if consent.Purpose != "account_delete" {
		t.Fatal("missing deletion consent purpose")
	}
	var binding *http.Cookie
	for _, c := range out.Result().Cookies() {
		if c.Name == nativeBrowserCookie {
			binding = c
		}
	}
	form := url.Values{"request": {handle}, "csrf_token": {consent.CSRFToken}, "decision": {"approve"}, "remember_me": {"false"}}
	request = httptest.NewRequest("POST", "/api/auth/native/authorize", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "https://example.test")
	request.AddCookie(browserCookie)
	request.AddCookie(binding)
	out = httptest.NewRecorder()
	mux.ServeHTTP(out, request)
	if out.Code != 303 {
		t.Fatalf("approve %d %s", out.Code, out.Body.String())
	}
	callback, _ := url.Parse(out.Header().Get("Location"))
	if len(callback.Query()) != 3 {
		t.Fatal("callback changed")
	}
	body, _ = json.Marshal(api.NativeAuthExchange{ClientID: input.ClientID, RedirectURI: input.RedirectURI, Code: callback.Query().Get("code"), CodeVerifier: nativeVerifier})
	request = httptest.NewRequest("POST", "/api/auth/native/exchange", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	out = httptest.NewRecorder()
	mux.ServeHTTP(out, request)
	if out.Code != 200 || strings.TrimSpace(out.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("exchange %d %s", out.Code, out.Body.String())
	}
	nativeJTI := responseSessionID(t, h, out)
	if nativeJTI == browserJTI || !h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, nativeJTI, hash) || h.ctx.AuthState.ConsumeAccountDeletionProof(attempt, id, browserJTI, hash) {
		t.Fatal("native/browser proof boundary")
	}
}

func deletionJSONRequest(path string, body []byte) *http.Request {
	r := httptest.NewRequest("POST", path, strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestAccountDeletionSSOAppliesLocalMFAAndResumeNeverMints(t *testing.T) {
	for _, publicURL := range []string{"https://example.test/prefix", "http://localhost:8080/prefix"} {
		t.Run(publicURL, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			provider := newFakeOIDCProvider(t, "delete-sso@example.com", true)
			h.ctx.SSO = sso.NewClient(provider.server.Client())
			team := configureTestSSO(t, h, store, provider.issuerURL())
			id := addTestSSOMember(t, store, team, "delete-sso@example.com")
			h.ctx.Config.PublicURL = publicURL
			parsedIssuer, _ := url.Parse(publicURL)
			origin := parsedIssuer.Scheme + "://" + parsedIssuer.Host
			callback := func(returnPath string) *httptest.ResponseRecorder {
				authorization := startTestSSOLogin(t, h, "delete-sso@example.com", returnPath, false)
				u, _ := url.Parse(authorization)
				provider.setNonce(u.Query().Get("nonce"))
				out := httptest.NewRecorder()
				h.handleSSOCallback()(out, httptest.NewRequest("GET", "/api/auth/sso/callback?state="+url.QueryEscape(u.Query().Get("state"))+"&code=test-code", nil))
				return out
			}
			// Establish the existing linked subject through a real signed OIDC completion.
			ordinary := callback("/events")
			if ordinary.Code != 303 {
				t.Fatal("ordinary OIDC failed")
			}
			responseSessionID(t, h, ordinary)
			const secret = "JBSWY3DPEHPK3PXP"
			if err := store.EnableUserTOTP(t.Context(), id, secret); err != nil {
				t.Fatal(err)
			}
			hash := strings.Repeat("a", 64)
			attempt := h.ctx.AuthState.CreateAccountDeletionAttempt(id, shared.AccountDeletionIssuer(publicURL), hash, "/settings", time.Now().Add(time.Minute))
			if attempt == "" {
				t.Fatal("browser deletion attempt missing")
			}
			target := shared.AccountDeletionReturnPath(attempt)
			out := callback(target)
			if out.Code != 303 {
				t.Fatalf("callback %d %s", out.Code, out.Body.String())
			}
			u, _ := url.Parse(out.Header().Get("Location"))
			fragment, _ := url.ParseQuery(u.Fragment)
			challenge := fragment.Get("mfa_challenge")
			if challenge == "" || u.Query().Get("returnUrl") != target {
				t.Fatalf("missing local MFA return: %s", u.Path)
			}
			var browser *http.Cookie
			for _, c := range out.Result().Cookies() {
				if c.Name == authcore.CookieName || c.Name == authcore.RememberMeCookieName {
					t.Fatal("OIDC skipped configured MFA")
				}
				if c.Name == mfaResumeCookie {
					browser = c
				}
			}
			if browser == nil || !browser.HttpOnly || browser.Secure != (parsedIssuer.Scheme == "https") {
				t.Fatal("missing browser binding")
			}
			body, _ := json.Marshal(map[string]string{"challenge_id": challenge})
			resume := func(cookie *http.Cookie, origin string, body []byte) *httptest.ResponseRecorder {
				r := deletionJSONRequest("/api/auth/mfa/resume", body)
				if cookie != nil {
					r.AddCookie(cookie)
				}
				if origin != "" {
					r.Header.Set("Origin", origin)
				}
				rr := httptest.NewRecorder()
				h.handleMFAResume()(rr, r)
				return rr
			}
			for _, tc := range []struct {
				cookie *http.Cookie
				origin string
			}{{nil, origin}, {browser, ""}, {browser, "https://evil.test"}, {&http.Cookie{Name: mfaResumeCookie, Value: "wrong"}, origin}} {
				if rr := resume(tc.cookie, tc.origin, body); rr.Code == 200 || len(rr.Result().Cookies()) != 0 {
					t.Fatal("invalid resume accepted")
				}
			}
			for _, invalid := range []string{`{"challenge_id":"` + challenge + `","challenge_id":"` + challenge + `"}`, `{"challenge_id":"` + challenge + `","user_id":"other"}`} {
				if rr := resume(browser, origin, []byte(invalid)); rr.Code != 400 {
					t.Fatal("non-strict resume")
				}
			}
			resumed := resume(browser, origin, body)
			if resumed.Code != 200 || len(resumed.Result().Cookies()) != 0 {
				t.Fatalf("resume %d %s", resumed.Code, resumed.Body.String())
			}
			var response loginResponse
			if err := json.Unmarshal(resumed.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Status != "mfa_required" || response.ChallengeToken != challenge || len(response.Factors) != 1 || response.Factors[0] != "totp" {
				t.Fatal("resume changed factors")
			}
			if h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, "current", hash) {
				t.Fatal("resume proved deletion")
			}
			// Cancellation leaves the original challenge available and emits no credentials.
			if rr := resume(browser, origin, body); rr.Code != 200 || len(rr.Result().Cookies()) != 0 {
				t.Fatal("resume cannot recover cancellation")
			}
			code, err := security.GenerateCurrentTOTPCode(secret, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			body, _ = json.Marshal(mfaTotpVerifyRequest{ChallengeToken: challenge, Code: code})
			completed := httptest.NewRecorder()
			h.handleMFATOTPVerify()(completed, deletionJSONRequest("/api/auth/mfa/totp/verify", body))
			if completed.Code != 200 || !h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, responseSessionID(t, h, completed), hash) {
				t.Fatalf("final factor %d %s", completed.Code, completed.Body.String())
			}
			if rr := resume(browser, origin, []byte(`{"challenge_id":"`+challenge+`"}`)); rr.Code == 200 {
				t.Fatal("spent challenge resumed")
			}
			// Ordinary SSO keeps its established direct-session policy even with local MFA.
			ordinary = callback("/events")
			if ordinary.Code != 303 {
				t.Fatal("ordinary SSO changed")
			}
			responseSessionID(t, h, ordinary)
		})
	}
}

func TestAccountDeletionPrimaryRequiresConfiguredFactors(t *testing.T) {
	for _, factor := range []string{"totp", "recovery_code", "email_link", "passkey"} {
		t.Run(factor, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			password, err := HashPassword("password123")
			if err != nil {
				t.Fatal(err)
			}
			id, err := store.CreateUser(t.Context(), "delete-factor@example.test", password)
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
				if _, err := store.CreateUserPasskeyCredential(t.Context(), id, "Deletion factor", fixture.Credential()); err != nil {
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
			var driver *authTestMailDriver
			if factor == "email_link" {
				driver = &authTestMailDriver{}
				h.ctx.Mailer = mailer.NewWithDriver(driver, h.ctx.Config)
			}
			attempt, hash := deletionAttemptFixture(t, h, id)
			target := shared.AccountDeletionReturnPath(attempt)
			body, _ := json.Marshal(map[string]string{"email": "delete-factor@example.test", "password": "password123", "return_url": target})
			out := httptest.NewRecorder()
			h.handleLogin()(out, deletionJSONRequest("/api/login", body))
			if out.Code != 200 || len(out.Result().Cookies()) != 0 {
				t.Fatalf("primary %d %s", out.Code, out.Body.String())
			}
			var response loginResponse
			if err := json.Unmarshal(out.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Status != "mfa_required" {
				t.Fatal("primary skipped MFA")
			}
			body, _ = json.Marshal(mfaTotpVerifyRequest{ChallengeToken: response.ChallengeToken, Code: "invalid"})
			failed := httptest.NewRecorder()
			h.handleMFATOTPVerify()(failed, deletionJSONRequest("/api/auth/mfa/totp/verify", body))
			if failed.Code == 200 || len(failed.Result().Cookies()) != 0 {
				t.Fatal("invalid factor issued session")
			}
			completed := httptest.NewRecorder()
			switch factor {
			case "totp", "recovery_code":
				body, _ = json.Marshal(mfaTotpVerifyRequest{ChallengeToken: response.ChallengeToken, Code: code})
				if factor == "totp" {
					h.handleMFATOTPVerify()(completed, deletionJSONRequest("/api/auth/mfa/totp/verify", body))
				} else {
					h.handleMFARecoveryCodeVerify()(completed, deletionJSONRequest("/api/auth/mfa/recovery-code/verify", body))
				}
			case "passkey":
				credential, err := fixture.AssertionResponse(response.Passkey.Challenge, "https://example.test", "example.test", id[:], 1, true)
				if err != nil {
					t.Fatal(err)
				}
				body, _ = json.Marshal(map[string]any{"challenge_token": response.ChallengeToken, "credential": credential, "return_url": target})
				h.handlePasskeyLoginFinish()(completed, deletionJSONRequest("/api/auth/passkey/login/finish", body))
			case "email_link":
				body, _ = json.Marshal(mfaEmailLinkRequest{ChallengeToken: response.ChallengeToken, ReturnURL: "/other"})
				mail := httptest.NewRecorder()
				h.handleMFAEmailLinkRequest()(mail, deletionJSONRequest("/api/auth/mfa/email-link/request", body))
				if mail.Code != 200 {
					t.Fatalf("mail %d %s", mail.Code, mail.Body.String())
				}
				token := extractMagicLinkToken(t, driver.textBody)
				h.handleMFAEmailLinkVerify()(completed, httptest.NewRequest("GET", "/api/auth/mfa/email-link/verify?token="+token, nil))
				if completed.Header().Get("Location") != "https://example.test/prefix"+target {
					t.Fatal("cross-browser email lost frozen action")
				}
			}
			if completed.Code != 200 && completed.Code != 303 {
				t.Fatalf("factor %d %s", completed.Code, completed.Body.String())
			}
			if !h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, responseSessionID(t, h, completed), hash) {
				t.Fatal("verified factor did not prove deletion")
			}
		})
	}
}

func TestAccountDeletionPasskeyPrimaryRequiresUVAndFrozenAttempt(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUser(t.Context(), "delete-passkey@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := testutil.NewPasskeyFixture()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUserPasskeyCredential(t.Context(), id, "Deletion primary", fixture.Credential()); err != nil {
		t.Fatal(err)
	}
	attempt, hash := deletionAttemptFixture(t, h, id)
	target := shared.AccountDeletionReturnPath(attempt)
	for _, uv := range []bool{false, true} {
		body, _ := json.Marshal(map[string]string{"return_url": target})
		start := httptest.NewRecorder()
		h.handlePasskeyLoginStart()(start, deletionJSONRequest("/api/auth/passkey/login/start", body))
		if start.Code != 200 {
			t.Fatal(start.Body.String())
		}
		var challenge passkeyLoginStartResponse
		if err := json.Unmarshal(start.Body.Bytes(), &challenge); err != nil {
			t.Fatal(err)
		}
		credential, err := fixture.AssertionResponse(challenge.PublicKey.Challenge, "https://example.test", "example.test", id[:], 1, uv)
		if err != nil {
			t.Fatal(err)
		}
		body, _ = json.Marshal(map[string]any{"challenge_token": challenge.ChallengeToken, "credential": credential, "return_url": target})
		out := httptest.NewRecorder()
		h.handlePasskeyLoginFinish()(out, deletionJSONRequest("/api/auth/passkey/login/finish", body))
		if !uv {
			if out.Code == 200 || len(out.Result().Cookies()) != 0 {
				t.Fatal("passkey without UV accepted")
			}
			continue
		}
		if out.Code != 200 || !h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, responseSessionID(t, h, out), hash) {
			t.Fatalf("UV passkey %d %s", out.Code, out.Body.String())
		}
	}
}

func TestAccountDeletionPhaseGuardsEveryHumanIssuance(t *testing.T) {
	for _, state := range []string{"removing", "complete", "database-fault"} {
		t.Run(state, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			password, err := HashPassword("password123")
			if err != nil {
				t.Fatal(err)
			}
			id, err := store.CreateUser(t.Context(), "delete-guard@example.test", password)
			if err != nil {
				t.Fatal(err)
			}
			deletionAttemptFixture(t, h, id)
			remembered, err := store.CreateRememberMeToken(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			current := nativeUserCookie(t, h, id)
			handle, mux := nativeStartFixture(t, h)
			_, fingerprint, err := h.nativeAccountSecurity(httptest.NewRequest("GET", "/", nil), id)
			if err != nil {
				t.Fatal(err)
			}
			csrf, _, ok := h.ctx.AuthState.RenderNativeAuth(handle, "browser", id, fingerprint)
			if !ok {
				t.Fatal("render")
			}
			code, ok := h.ctx.AuthState.DecideNativeAuth(handle, "browser", csrf, id, fingerprint, true, false)
			if !ok {
				t.Fatal("approve")
			}
			if state == "database-fault" {
				if err := store.Exec(t.Context(), "DROP TABLE account_deletion_intents"); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := store.Exec(t.Context(), `INSERT INTO account_deletion_intents (attempt_id,user_id,consequence_hash,choices,consequences,cloud,phase,expires_at) VALUES (?,?,?,'{}','{}',FALSE,?,?)`, "fixture", id, strings.Repeat("a", 64), state, time.Now().Add(time.Minute)); err != nil {
					t.Fatal(err)
				}
			}
			assertNoMint := func(out *httptest.ResponseRecorder) {
				t.Helper()
				if out.Code < 400 {
					t.Fatal("removing account authorized")
				}
				for _, cookie := range out.Result().Cookies() {
					if cookie.Value != "" && cookie.MaxAge >= 0 {
						t.Fatal("removing account issued credentials")
					}
				}
			}
			for _, cookie := range []*http.Cookie{current, {Name: authcore.RememberMeCookieName, Value: remembered}} {
				out := httptest.NewRecorder()
				req := httptest.NewRequest("GET", "/api/auth/session", nil)
				req.AddCookie(cookie)
				mux.ServeHTTP(out, req)
				assertNoMint(out)
			}
			body, _ := json.Marshal(map[string]string{"email": "delete-guard@example.test", "password": "password123"})
			out := httptest.NewRecorder()
			h.handleLogin()(out, deletionJSONRequest("/api/login", body))
			assertNoMint(out)
			out = httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/auth/session/extend", nil)
			req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, id))
			h.handleExtendSession()(out, req)
			assertNoMint(out)
			body, _ = json.Marshal(api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, Code: code, CodeVerifier: nativeVerifier})
			out = httptest.NewRecorder()
			mux.ServeHTTP(out, deletionJSONRequest("/api/auth/native/exchange", body))
			assertNoMint(out)
		})
	}
}

func TestAccountDeletionNativeStartPurposeIsPairedAndLoginOnly(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	_, mux := nativeStartFixture(t, h)
	digest := sha256.Sum256([]byte(nativeVerifier))
	base := map[string]any{"client_id": "hitkeep-ios", "redirect_uri": shared.NativeIOSCallback, "code_challenge": base64.RawURLEncoding.EncodeToString(digest[:]), "code_challenge_method": "S256", "state": "native-test-state", "intent": "login"}
	for _, extra := range []map[string]any{{"purpose": "other", "reauth_attempt": "attempt"}, {"purpose": "account_delete"}, {"reauth_attempt": "attempt"}, {"purpose": "", "reauth_attempt": ""}, {"purpose": "account_delete", "reauth_attempt": "attempt", "intent": "signup"}} {
		input := maps.Clone(base)
		maps.Copy(input, extra)
		body, _ := json.Marshal(input)
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, deletionJSONRequest("/api/auth/native/start", body))
		if out.Code != 400 || strings.TrimSpace(out.Body.String()) != `{"error":"invalid_request"}` {
			t.Fatalf("purpose accepted %d %s", out.Code, out.Body.String())
		}
	}
}

func TestAccountDeletionPrimaryStartsRejectForeignOriginAndDelegatedHeaders(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	id, err := store.CreateUser(t.Context(), "delete-origin@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	attempt, _ := deletionAttemptFixture(t, h, id)
	h.ctx.Config.SocialGoogleClientID = "fixture-client"
	h.ctx.Config.SocialGoogleClientSecret = "fixture-secret"
	h.ctx.SocialAuth = socialauth.NewClient(nil)
	for _, method := range []string{"password", "sso", "social", "passkey"} {
		for _, header := range []string{"Origin", "Authorization", "X-Api-Key", "X-Hitkeep-Agent"} {
			input := map[string]any{"return_url": shared.AccountDeletionReturnPath(attempt)}
			if method == "social" {
				input["flow"] = "login"
			}
			body, _ := json.Marshal(input)
			req := deletionJSONRequest("/api/login", body)
			req.Header.Set(header, "https://evil.test")
			out := httptest.NewRecorder()
			switch method {
			case "password":
				h.handleLogin()(out, req)
			case "sso":
				h.handleSSOStart()(out, req)
			case "social":
				req.SetPathValue("provider", "google")
				h.handleSocialStart(false)(out, req)
			case "passkey":
				h.handlePasskeyLoginStart()(out, req)
			}
			if out.Code != 403 || len(out.Result().Cookies()) != 0 {
				t.Fatalf("%s %s status=%d", method, header, out.Code)
			}
		}
	}
}

func TestAccountDeletionHTTPBrowserProofDoesNotEnableNative(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.PublicURL = "http://localhost:8080"
	password, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateUser(t.Context(), "http-proof@example.test", password)
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	attempt := h.ctx.AuthState.CreateAccountDeletionAttempt(id, h.ctx.Config.PublicURL, hash, "/settings", time.Now().Add(time.Minute))
	if attempt == "" {
		t.Fatal("HTTP browser deletion unavailable")
	}
	body, _ := json.Marshal(map[string]string{"email": "http-proof@example.test", "password": "password123", "return_url": shared.AccountDeletionReturnPath(attempt)})
	req := deletionJSONRequest("/api/login", body)
	req.Header.Set("Origin", "http://localhost:8080")
	out := httptest.NewRecorder()
	h.handleLogin()(out, req)
	if out.Code != 200 || !h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, responseSessionID(t, h, out), hash) {
		t.Fatalf("HTTP primary %d %s", out.Code, out.Body.String())
	}
	if h.ctx.NativeAuthReady() || h.ctx.NativeAuthStatus() != nil {
		t.Fatal("native advertised over HTTP")
	}
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	out = httptest.NewRecorder()
	mux.ServeHTTP(out, deletionJSONRequest("/api/auth/native/start", []byte(`{}`)))
	if out.Code != 503 {
		t.Fatal("native HTTP start accepted")
	}
}

func TestOrdinarySSONewIdentityInvalidatesExistingDeletionProof(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "sso-link@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	team := configureTestSSO(t, h, store, provider.issuerURL())
	password, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "sso-link@example.com", password)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTeamMember(t.Context(), team, id, database.TenantRoleMember, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	attempt, hash, jti := freshSocialMutationProof(t, h, id, "sso-link@example.com")
	authorization := startTestSSOLogin(t, h, "sso-link@example.com", "/dashboard", false)
	u, _ := url.Parse(authorization)
	provider.setNonce(u.Query().Get("nonce"))
	out := httptest.NewRecorder()
	h.handleSSOCallback()(out, httptest.NewRequest("GET", "/api/auth/sso/callback?state="+url.QueryEscape(u.Query().Get("state"))+"&code=test-code", nil))
	if out.Code != 303 {
		t.Fatalf("SSO callback %d", out.Code)
	}
	responseSessionID(t, h, out)
	if h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, jti, hash) {
		t.Fatal("new SSO primary retained deletion proof")
	}
	// A later normal login using the same subject is usage-only.
	attempt, hash, jti = freshSocialMutationProof(t, h, id, "sso-link@example.com")
	authorization = startTestSSOLogin(t, h, "sso-link@example.com", "/dashboard", false)
	u, _ = url.Parse(authorization)
	provider.setNonce(u.Query().Get("nonce"))
	out = httptest.NewRecorder()
	h.handleSSOCallback()(out, httptest.NewRequest("GET", "/api/auth/sso/callback?state="+url.QueryEscape(u.Query().Get("state"))+"&code=test-code", nil))
	responseSessionID(t, h, out)
	if !h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, jti, hash) {
		t.Fatal("ordinary SSO usage invalidated proof")
	}
}
