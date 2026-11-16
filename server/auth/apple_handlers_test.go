package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
	"hitkeep/socialauth"
)

func TestApplePOSTReceiptRestoresBrowserBindingAndIsOneUse(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.AuthState = shared.NewAuthStateStore()
	h.ctx.SocialAuth = socialauth.NewClient(nil)
	state := h.ctx.AuthState.CreateSocialOAuthState(shared.SocialOAuthState{Provider: "apple", Flow: "login", ExpiresAt: time.Now().Add(time.Minute)})
	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/social/apple/callback", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.handleApplePOSTCallback().ServeHTTP(w, r)
		return w
	}
	response := post(url.Values{"state": {state}, "error": {"access_denied"}}.Encode())
	if response.Code != http.StatusSeeOther || strings.Contains(response.Header().Get("Location"), state) {
		t.Fatalf("receipt: %d %s", response.Code, response.Body.String())
	}
	if repeated := post(url.Values{"state": {state}, "error": {"access_denied"}}.Encode()); repeated.Code != http.StatusBadRequest {
		t.Fatal("duplicate POST accepted")
	}
	resumeURL := response.Header().Get("Location")
	r := httptest.NewRequest(http.MethodGet, resumeURL, nil)
	w := httptest.NewRecorder()
	h.handleAppleResume().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatal("wrong browser accepted")
	}
	cookies := httptest.NewRecorder()
	h.setSocialStateCookie(cookies, state)
	for _, cookie := range cookies.Result().Cookies() {
		r.AddCookie(cookie)
	}
	w = httptest.NewRecorder()
	h.handleAppleResume().ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "social_provider_cancelled") {
		t.Fatalf("original browser: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.handleAppleResume().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatal("receipt replay accepted")
	}
	for _, body := range []string{"state=x&state=y&code=z", "state=x&code=c&error=e", "state=x&code=c&user=[]", "state=x&code=" + strings.Repeat("x", 17000)} {
		if w := post(body); w.Code != http.StatusBadRequest {
			t.Fatalf("malformed body accepted: %d", w.Code)
		}
	}
	r = httptest.NewRequest(http.MethodGet, "/api/auth/social/apple/callback?state="+state+"&code=direct", nil)
	r.SetPathValue("provider", "apple")
	w = httptest.NewRecorder()
	h.handleSocialCallback().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatal("direct Apple GET accepted")
	}
}

func TestAppleImmutableLoginBeforeEmailAndRevocationFallback(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.AuthState = shared.NewAuthStateStore()
	userID, err := store.CreateUser(t.Context(), "original@private.icloud.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.LinkSocialIdentity(t.Context(), database.LinkSocialIdentityInput{UserID: userID, Provider: "apple", Subject: "stable", ObservedEmail: "original@private.icloud.com"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/auth/social/complete", nil)
	h.completeSocialLogin(w, r, socialCompletionRequest{}, shared.SocialCompletion{Provider: "apple", Subject: "stable", Flow: "login", ReturnPath: "/dashboard"})
	if w.Code != http.StatusOK {
		t.Fatalf("immutable repeated login without email: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.completeSocialLogin(w, r, socialCompletionRequest{}, shared.SocialCompletion{Provider: "apple", Subject: "new", ObservedEmail: "original@private.icloud.com", EmailVerified: false})
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "social_email_unverified") {
		t.Fatal("unverified email linked")
	}
	h.ctx.SocialAuth = socialauth.NewClient(nil)
	r = httptest.NewRequest(http.MethodDelete, "/api/user/security/social/apple", strings.NewReader(`{"current_password":""}`))
	r.SetPathValue("provider", "apple")
	r = r.WithContext(context.WithValue(r.Context(), shared.UserIDKey, userID))
	w = httptest.NewRecorder()
	h.handleSocialUnlink().ServeHTTP(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("last primary Apple unlink accepted: %d", w.Code)
	}
	if _, err := store.LinkSocialIdentity(t.Context(), database.LinkSocialIdentityInput{UserID: userID, Provider: "google", Subject: "alternative"}); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodDelete, "/api/user/security/social/apple", strings.NewReader(`{}`))
	r.SetPathValue("provider", "apple")
	r = r.WithContext(context.WithValue(r.Context(), shared.UserIDKey, userID))
	w = httptest.NewRecorder()
	h.handleSocialUnlink().ServeHTTP(w, r)
	var response struct {
		Status              string `json:"status"`
		ProviderRevocations []struct {
			Status string `json:"status"`
		} `json:"provider_revocations"`
	}
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || w.Code != http.StatusOK || len(response.ProviderRevocations) != 1 || response.ProviderRevocations[0].Status != "manual_revocation_required" {
		t.Fatalf("Apple unlink fallback: %d %s", w.Code, w.Body.String())
	}
	if identity, err := store.GetUserAppleIdentity(t.Context(), userID); err != nil || identity != nil {
		t.Fatal("local Apple identity retained after fallback")
	}

}

// TestApplePOSTRealBrowserFixture is an opt-in finite local fixture for an actual browser's SameSite behavior.
func TestApplePOSTRealBrowserFixture(t *testing.T) {
	artifact := os.Getenv("HITKEEP_APPLE_BROWSER_FIXTURE_FILE")
	if artifact == "" {
		t.Skip("real-browser fixture requested explicitly")
	}
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.AuthState = shared.NewAuthStateStore()
	h.ctx.SocialAuth = socialauth.NewClient(nil)
	var state string
	postHadCookie, resumeHadCookie := false, false
	done := make(chan struct{})
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	h.ctx.Config.PublicURL = server.URL
	mux.HandleFunc("GET /start", func(w http.ResponseWriter, r *http.Request) {
		state = h.ctx.AuthState.CreateSocialOAuthState(shared.SocialOAuthState{Provider: "apple", Flow: "login", ExpiresAt: time.Now().Add(time.Minute)})
		h.setSocialStateCookie(w, state)
		http.Redirect(w, r, strings.Replace(server.URL, "127.0.0.1", "localhost", 1)+"/provider", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /provider", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!doctype html><title>Local Apple receipt fixture</title><form method="post" action="%s/api/auth/social/apple/callback"><input type="hidden" name="state" value="%s"><input type="hidden" name="error" value="access_denied"><button>Continue fixture</button></form>`, server.URL, state)
	})
	mux.HandleFunc("POST /api/auth/social/apple/callback", func(w http.ResponseWriter, r *http.Request) {
		postHadCookie = h.hasSocialStateCookie(r, state)
		h.handleApplePOSTCallback().ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /api/auth/social/apple/resume", func(w http.ResponseWriter, r *http.Request) {
		resumeHadCookie = h.hasSocialStateCookie(r, state)
		h.handleAppleResume().ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("error") != "social_provider_cancelled" || postHadCookie || !resumeHadCookie {
			http.Error(w, "Fixture failed", 500)
			return
		}
		_, _ = fmt.Fprint(w, "PASS: cross-site POST omitted Lax cookie; same-origin GET restored original browser proof; one-use callback completed safely.")
		close(done)
	})
	if err := os.WriteFile(artifact, []byte(server.URL+"/start"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("real-browser fixture timed out")
	}
}

func TestAppleSubjectLoginStillRequiresLocalMFA(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	userID, err := store.CreateUser(t.Context(), "mfa@private.icloud.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnableUserTOTP(t.Context(), userID, "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LinkSocialIdentity(t.Context(), database.LinkSocialIdentityInput{UserID: userID, Provider: "apple", Subject: "mfa-sub"}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/auth/social/complete", nil)
	h.completeSocialLogin(w, r, socialCompletionRequest{}, shared.SocialCompletion{Provider: "apple", Subject: "mfa-sub", RememberMe: true, ReturnPath: "/dashboard"})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"mfa_required"`) {
		t.Fatalf("Apple bypassed local MFA: %d %s", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("session issued before MFA")
	}
}

func TestAppleRelaySignupPreservesTermsAndEncryptedCredential(t *testing.T) {
	for _, domain := range []string{"private.icloud.com", "privaterelay.appleid.com"} {
		t.Run(domain, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			h.ctx.Config.CloudHosted = true
			h.ctx.Config.CloudSignupEnabled = true
			h.ctx.Config.SocialSignupEnabled = true
			h.ctx.Config.CloudJurisdiction = "EU"
			key := strings.Repeat("k", 32)
			ciphertext, err := socialauth.SealAppleRefreshToken(key, "service", "relay-sub", "refresh-fixture")
			if err != nil {
				t.Fatal(err)
			}
			completion := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{Provider: "apple", Subject: "relay-sub", ObservedEmail: "alias@" + domain, EmailVerified: true, Flow: "signup", AppleClientID: "service", AppleRefreshTokenCiphertext: ciphertext, ExpiresAt: time.Now().Add(time.Minute)})
			request := map[string]any{"completion_token": completion, "team_name": "Relay Team", "plan_code": "free", "billing": "monthly", "jurisdiction": "EU", "locale": "en", "accepted_tos": false}
			body, _ := json.Marshal(request)
			w := httptest.NewRecorder()
			h.handleSocialCloudSignupComplete().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/cloud/signup/social/complete", strings.NewReader(string(body))))
			if w.Code != http.StatusBadRequest {
				t.Fatal("Apple bypassed Terms")
			}
			request["accepted_tos"] = true
			body, _ = json.Marshal(request)
			w = httptest.NewRecorder()
			h.handleSocialCloudSignupComplete().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/cloud/signup/social/complete", strings.NewReader(string(body))))
			if w.Code != http.StatusCreated {
				t.Fatalf("relay signup: %d %s", w.Code, w.Body.String())
			}
			identity, err := store.GetSocialIdentity(t.Context(), "apple", "relay-sub")
			if err != nil || identity == nil || identity.AppleRefreshTokenCiphertext != ciphertext {
				t.Fatalf("encrypted Apple credential lost: %v", err)
			}
			if strings.Contains(w.Body.String(), "refresh-fixture") || strings.Contains(w.Body.String(), ciphertext) {
				t.Fatal("credential in signup response")
			}
		})
	}
}
