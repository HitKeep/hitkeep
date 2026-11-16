package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"hitkeep/entitlements"
	"hitkeep/server/shared"
)

func TestServerApplePOSTReceiptPreservesPrefixAndBrowserProof(t *testing.T) {
	conf := testServerConfig(t)
	conf.PublicURL = "https://www.example.net/hitkeep"
	store := testServerStore(t)
	defer store.Close()
	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil, testServerLogger())
	defer func() { _ = srv.Shutdown(t.Context()) }()
	state := srv.ctx.AuthState.CreateSocialOAuthState(shared.SocialOAuthState{Provider: "apple", Flow: "login", ExpiresAt: time.Now().Add(time.Minute)})
	form := url.Values{"state": {state}, "error": {"access_denied"}}
	request := httptest.NewRequest(http.MethodPost, conf.PublicURL+"/api/auth/social/apple/callback", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	request.Header.Set("Origin", "https://appleid.apple.com")
	response := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || !strings.HasPrefix(response.Header().Get("Location"), conf.PublicURL+"/api/auth/social/apple/resume?receipt=") {
		t.Fatalf("Apple ingress/prefix: %d %s", response.Code, response.Body.String())
	}
	resume := httptest.NewRequest(http.MethodGet, response.Header().Get("Location"), nil)
	response = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(response, resume)
	if response.Code != http.StatusBadRequest {
		t.Fatal("middleware allowed wrong browser")
	}
	resume.AddCookie(&http.Cookie{Name: "hk_social_state_" + state, Value: state})
	response = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(response, resume)
	if response.Code != http.StatusSeeOther || !strings.Contains(response.Header().Get("Location"), "/hitkeep/login?error=social_provider_cancelled") {
		t.Fatalf("same-browser prefixed resume: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(response, resume)
	if response.Code != http.StatusBadRequest {
		t.Fatal("middleware allowed receipt replay")
	}
}
