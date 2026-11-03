package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	stdUUID "uuid"

	"github.com/google/uuid"
	"hitkeep/analyticstools"
	"hitkeep/auth"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/oauth"
	"hitkeep/server/shared"
)

func TestA2ASynchronousRegistryExecutionAndDenial(t *testing.T) {
	siteID := uuid.New()
	scope := analyticstools.Scope{MaxRangeDays: 31, Resolve: func(context.Context, uuid.UUID) (analyticstools.Site, error) {
		return analyticstools.Site{}, shared.ErrOAuthAccessDenied
	}}
	for _, tc := range []struct {
		name, body, version string
		want                int
	}{
		{"version", `{}`, "0.3", 400},
		{"missing version", `{}`, "", 400},
		{"oversized", strings.Repeat(" ", 64<<10) + `{}`, "1.0", 400},
		{"duplicate member", `{"message":{},"message":{}}`, "1.0", 400},
		{"unsupported task", `{"message":{"taskId":"unknown"}}`, "1.0", 404},
		{"unsupported push", `{"configuration":{"taskPushNotificationConfig":{"url":"https://untrusted.example/"}}}`, "1.0", 400},
		{"unsupported output", `{"configuration":{"acceptedOutputModes":["text/plain"]}}`, "1.0", 400},
		{"bounded range", `{"message":{"messageId":"1","role":"ROLE_USER","parts":[{"data":{"tool":"hitkeep_get_site_overview","arguments":{"site_id":"` + siteID.String() + `","from":"2020-01-01T00:00:00Z","to":"2021-01-01T00:00:00Z"}}}]}}`, "1.0", 400},
		{"malformed", `{"message":`, "1.0", 400},
		{"text", `{"message":{"messageId":"1","role":"ROLE_USER","parts":[{"text":"hello"}]}}`, "1.0", 400},
		{"unknown tool", `{"message":{"messageId":"1","role":"ROLE_USER","parts":[{"data":{"tool":"delete_site","arguments":{}}}]}}`, "1.0", 400},
		{"denied site", `{"message":{"messageId":"1","role":"ROLE_USER","parts":[{"data":{"tool":"hitkeep_get_event_names","arguments":{"site_id":"` + siteID.String() + `"}}}]}}`, "1.0", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/a2a/message:send", strings.NewReader(tc.body))
			req.Header.Set("A2A-Version", tc.version)
			w := httptest.NewRecorder()
			serveA2AMessage(w, req, scope, analyticstools.Analytics())
			if w.Code != tc.want {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), `"role":"ROLE_AGENT"`) {
				t.Fatal("denied request returned analytics")
			}
		})
	}
	tool := analyticstools.Define("test_read", "Read test aggregate", "Read test aggregate", func(context.Context, analyticstools.Call, struct{}) (map[string]int, error) {
		return map[string]int{"count": 42}, nil
	})
	req := httptest.NewRequest("POST", "/api/a2a/message:send", strings.NewReader(`{"message":{"messageId":"1","role":"ROLE_USER","parts":[{"data":{"tool":"test_read","arguments":{}}}]}}`))
	req.Header.Set("A2A-Version", "1.0")
	w := httptest.NewRecorder()
	serveA2AMessage(w, req, scope, []analyticstools.Tool{tool})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"count":42`) || !strings.Contains(w.Body.String(), `"role":"ROLE_AGENT"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestA2AFlagAndOAuthBoundary(t *testing.T) {
	s := newDiscoveryServer()
	s.ctx.Store = testServerStore(t)
	s.ctx.TenantStores = database.NewTenantStoreManager(s.ctx.Store, t.TempDir(), database.WithTenantDataPlane(false))
	t.Cleanup(func() { _ = s.ctx.Store.Close() })
	provider, err := oauth.New(s.ctx.Store, oauth.Options{Issuer: s.conf.PublicURL, MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("s", 32)), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	s.ctx.OAuthProvider = provider
	mux := http.NewServeMux()
	s.registerA2ARoutes(mux)
	for _, enabled := range []bool{false, true} {
		s.conf.AIEnabled = enabled
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/a2a/message:send", strings.NewReader(`{}`))
		req.AddCookie(&http.Cookie{Name: "session", Value: "cookie-is-not-oauth"})
		mux.ServeHTTP(w, req)
		want := 404
		if enabled {
			want = 401
		}
		if w.Code != want {
			t.Fatalf("AI=%v status=%d", enabled, w.Code)
		}
	}
	grant := &auth.OAuthGrant{}
	scope := s.a2aScope(grant)
	if _, err := scope.Resolve(t.Context(), uuid.New()); err == nil {
		t.Fatal("unconsented site resolved")
	}
}

func TestRawAIStatusIndependentOfAskAI(t *testing.T) {
	s := newDiscoveryServer()
	s.ctx.Store = testServerStore(t)
	s.ctx.TenantStores = database.NewTenantStoreManager(s.ctx.Store, t.TempDir(), database.WithTenantDataPlane(false))
	t.Cleanup(func() { _ = s.ctx.Store.Close() })
	for _, enabled := range []bool{false, true} {
		s.conf.AIEnabled = enabled
		status, err := s.ctx.SystemStatusResponse(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		if status.AIEnabled != enabled || status.AskAI.Enabled || !strings.Contains(string(raw), `"ai_enabled":`) {
			t.Fatalf("status %s", raw)
		}
	}
}

func TestA2ARealAnalyticsConsentAndAuthority(t *testing.T) {
	s := newDiscoveryServer()
	s.ctx.Store = testServerStore(t)
	s.ctx.TenantStores = database.NewTenantStoreManager(s.ctx.Store, t.TempDir(), database.WithTenantDataPlane(false))
	t.Cleanup(func() { _ = s.ctx.Store.Close() })
	user, err := s.ctx.Store.CreateUser(t.Context(), "a2a@example.test", "unused-hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.ctx.Store.CreateSite(t.Context(), user, "a2a.example.test")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.ctx.Store.GetSiteTenantID(t.Context(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := oauth.New(s.ctx.Store, oauth.Options{Issuer: s.conf.PublicURL, MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("s", 32)), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	s.ctx.OAuthProvider = provider
	grant := auth.OAuthGrant{ID: stdUUID.New(), UserID: stdUUID.UUID(user), TeamID: stdUUID.UUID(team), SiteIDs: []stdUUID.UUID{stdUUID.UUID(site.ID)}, Scopes: []string{oauth.ScopeSitesRead, oauth.ScopeAnalyticsRead}, Audience: provider.Resource("api")}
	for _, tc := range []struct {
		name   string
		change func(*auth.OAuthGrant)
		want   int
	}{
		{"allowed", func(*auth.OAuthGrant) {}, 200},
		{"wrong audience", func(g *auth.OAuthGrant) { g.Audience = provider.Resource("mcp") }, 403},
		{"unconsented site", func(g *auth.OAuthGrant) { g.SiteIDs = nil }, 403},
		{"missing scope", func(g *auth.OAuthGrant) { g.Scopes = []string{oauth.ScopeSitesRead} }, 403},
		{"wrong team", func(g *auth.OAuthGrant) { g.TeamID = stdUUID.New() }, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := grant
			tc.change(&g)
			body := `{"message":{"messageId":"1","role":"ROLE_USER","parts":[{"data":{"tool":"hitkeep_get_site_overview","arguments":{"site_id":"` + site.ID.String() + `","sections":["kpis"]}}}]}}`
			req := httptest.NewRequest("POST", "/api/a2a/message:send", strings.NewReader(body))
			req.Header.Set("A2A-Version", "1.0")
			w := httptest.NewRecorder()
			serveA2AMessage(w, req, s.a2aScope(&g), analyticstools.Analytics())
			if w.Code != tc.want {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if tc.want == 200 && !strings.Contains(w.Body.String(), `"total_pageviews":0`) {
				t.Fatalf("no real aggregate: %s", w.Body.String())
			}
		})
	}
	for _, enabled := range []bool{true, false} {
		s.conf.AIEnabled = enabled
		card := discoveryRequest(s, "GET", "/.well-known/agent-card.json")
		want := 404
		if enabled {
			want = 200
		}
		if card.Code != want {
			t.Fatalf("AI=%v card=%d %s", enabled, card.Code, card.Body.String())
		}
	}
}
