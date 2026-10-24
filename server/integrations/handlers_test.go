package integrations

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/oauth"
	"hitkeep/server/shared"
)

func integrationFixture(t *testing.T) (*handler, *auth.OAuthGrant, uuid.UUID) {
	t.Helper()
	store := database.NewStore(filepath.Join(t.TempDir(), "integrations.db"))
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(t.Context(), "integration@example.test", "unused-hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(t.Context(), user, "integration.example")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.GetSiteTenantID(t.Context(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := oauth.New(store, oauth.Options{Issuer: "https://hitkeep.example/prefix", MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("b", 32)), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{ctx: &shared.Context{Store: store, Config: &config.Config{PublicURL: p.Issuer()}, OAuthProvider: p}}
	grant := &auth.OAuthGrant{ID: uuid.NewV7(), UserID: uuid.UUID(user), TeamID: uuid.UUID(team), SiteIDs: []uuid.UUID{uuid.UUID(site.ID)}, Scopes: []string{oauth.ScopeSitesRead, oauth.ScopeAnalyticsRead, oauth.ScopeTrackingDomainsRead}, Audience: p.Resource("api")}
	return h, grant, uuid.UUID(site.ID)
}
func integrationRequest(h *handler, g *auth.OAuthGrant, path string, siteID uuid.UUID) *http.Request {
	r := httptest.NewRequest(http.MethodGet, h.ctx.OAuthProvider.Issuer()+path, nil)
	r.SetPathValue("id", siteID.String())
	if g != nil {
		r = r.WithContext(context.WithValue(r.Context(), shared.OAuthGrantKey, g))
	}
	return r
}
func TestIntegrationDiscoveryFiltersConsentScopeAndResource(t *testing.T) {
	h, g, id := integrationFixture(t)
	for _, tc := range []struct {
		name   string
		change func(*auth.OAuthGrant)
		want   int
	}{
		{"allowed", func(*auth.OAuthGrant) {}, 1},
		{"unselected", func(g *auth.OAuthGrant) { g.SiteIDs = []uuid.UUID{uuid.NewV7()} }, 0},
		{"wrong scope", func(g *auth.OAuthGrant) { g.Scopes = []string{oauth.ScopeAnalyticsRead} }, 0},
		{"wrong resource", func(g *auth.OAuthGrant) { g.Audience = "https://hitkeep.example/mcp" }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			grant := *g
			tc.change(&grant)
			w := httptest.NewRecorder()
			h.sites(w, integrationRequest(h, &grant, "/api/integrations/sites", id))
			var result []api.IntegrationSite
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || len(result) != tc.want {
				t.Fatalf("discovery contract: %d %s %v", w.Code, w.Body.String(), err)
			}
		})
	}
}
func TestIntegrationTrackingDefaultAndBoundedPreview(t *testing.T) {
	h, g, id := integrationFixture(t)
	w := httptest.NewRecorder()
	h.trackingDomains(w, integrationRequest(h, g, "/api/integrations/sites/"+id.String()+"/tracking-domains", id))
	var result api.IntegrationTrackingDomains
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.DefaultScriptURL != "https://hitkeep.example/prefix/hk.js" || result.ActiveCustomDomains == nil {
		t.Fatalf("tracking contract: %d %s %v", w.Code, w.Body.String(), err)
	}
	w = httptest.NewRecorder()
	h.preview(w, integrationRequest(h, g, "/api/integrations/sites/"+id.String()+"/preview?raw=true", id))
	if w.Code != 400 {
		t.Fatalf("unbounded preview accepted: %d %s", w.Code, w.Body.String())
	}
}
func TestIntegrationTransportRejectsCookieOnlyIdentity(t *testing.T) {
	h, _, _ := integrationFixture(t)
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	r := httptest.NewRequest(http.MethodGet, "https://hitkeep.example/api/integrations/sites", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: "dashboard-session"})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("dashboard cookie reached integration transport: %d %s", w.Code, w.Body.String())
	}
}
