package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/database"
	"hitkeep/oauth"
	"hitkeep/server/shared"
)

func TestOAuthProtocolPathsDoNotReceiveDashboardShell(t *testing.T) {
	s := &Server{}
	for _, path := range []string{"/oauth/authorize", "/oauth/token", "/oauth/revoke"} {
		if !s.isNonDashboardPath(path) {
			t.Errorf("protocol path %q can receive dashboard shell", path)
		}
	}
	if s.isNonDashboardPath("/oauth/consent") {
		t.Fatal("consent must remain a dashboard route")
	}
}

func TestOAuthDiscoveryPassesOnlyExactOutsidePrefixPaths(t *testing.T) {
	p, err := oauth.New(&database.Store{}, oauth.Options{
		Issuer:            "https://analytics.example/analytics",
		MCPPath:           "/tools",
		MCPEnabled:        true,
		BrowserSigningKey: []byte(strings.Repeat("k", 32)),
		IsLeader:          func() bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{publicBasePath: "/analytics", ctx: &shared.Context{OAuthProvider: p}}
	handler := s.stripPublicBasePath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Observed-Path", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range p.DiscoveryPaths() {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNoContent || recorder.Header().Get("Observed-Path") != path {
			t.Errorf("canonical metadata path %q rejected or rewritten: %d", path, recorder.Code)
		}
	}
	for _, path := range []string{"/.well-known/unrelated", p.DiscoveryPaths()[0] + "/extra", "/oauth/token", "/oauth/consent"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("unrelated outside-prefix path %q allowed: %d", path, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analytics/oauth/consent", nil))
	if recorder.Code != http.StatusNoContent || recorder.Header().Get("Observed-Path") != "/oauth/consent" {
		t.Fatal("prefixed consent did not reach dashboard route")
	}
}
