package shared

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	legacyuuid "github.com/google/uuid"

	"hitkeep/auth"
	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/oauth"
)

func TestOAuthAuthenticationErrorIncludesConfiguredResourceMetadata(t *testing.T) {
	store := newSharedTestStore(t)
	defer store.Close()
	provider, err := oauth.New(store, oauth.Options{Issuer: "https://example.test", MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("k", 32)), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		provider  *oauth.Provider
		challenge string
	}{
		{"without provider", nil, `Bearer error="invalid_token"`},
		{"with provider", provider, `Bearer error="invalid_token", resource_metadata="https://example.test/.well-known/oauth-protected-resource/api"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Context{OAuthProvider: tc.provider}
			w := httptest.NewRecorder()
			c.writeOAuthAuthenticationError(w)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
			}
			if got := w.Header().Get("WWW-Authenticate"); got != tc.challenge {
				t.Fatalf("challenge = %q, want %q", got, tc.challenge)
			}
		})
	}
}

func TestRecognizedOAuthNeverFallsBackToDashboardCookies(t *testing.T) {
	c := &Context{Config: &config.Config{JWTSecret: "test-oauth-cookie-secret", PublicURL: "https://example.test"}}
	cookieToken, err := auth.GenerateToken(c.Config.JWTSecret, c.Config.PublicURL, legacyuuid.UUID(uuid.NewV7()))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	next := func(http.ResponseWriter, *http.Request) { called = true }
	for _, allowAPIKey := range []bool{false, true} {
		r := httptest.NewRequest(http.MethodGet, "/api/user", nil)
		r.Header.Set("Authorization", "Bearer hk_oauth_at_invalid")
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookieToken})
		w := httptest.NewRecorder()
		c.RequireAuth(allowAPIKey, next)(w, r)
		if w.Code != http.StatusUnauthorized || called {
			t.Fatalf("recognized OAuth fell through: %d, %v", w.Code, called)
		}
	}
}

func TestHumanOnlyRejectsBearerEvenWithValidSession(t *testing.T) {
	c := &Context{Config: &config.Config{JWTSecret: "test-human-only-secret", PublicURL: "https://example.test"}}
	token, err := auth.GenerateToken(c.Config.JWTSecret, c.Config.PublicURL, legacyuuid.UUID(uuid.NewV7()))
	if err != nil {
		t.Fatal(err)
	}
	for _, credential := range []string{"manual-client-token", "hk_oauth_at_invalid"} {
		r := httptest.NewRequest(http.MethodGet, "/api/user/oauth/grants", nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		r.Header.Set("Authorization", "Bearer "+credential)
		w := httptest.NewRecorder()
		c.Handler(HandlerConfig{RequireAuth: true, HumanOnly: true}, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("credential %q downgraded to session: %d", credential, w.Code)
		}
	}
}

func TestPermissionAdaptersPreserveDifferentAPIFallbacks(t *testing.T) {
	store := newSharedTestStore(t)
	defer store.Close()
	userID, err := store.CreateUser(t.Context(), "fallback@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateInstanceRole(t.Context(), userID, auth.InstanceOwner, userID); err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(t.Context(), userID, "fallback.example.test")
	if err != nil {
		t.Fatal(err)
	}
	c := &Context{Store: store}
	next := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	for _, tc := range []struct {
		name             string
		strict, hasGrant bool
		want             int
	}{
		{"legacy instance fallback", false, true, http.StatusNoContent},
		{"strict site denies fallback", true, true, http.StatusForbidden},
		{"missing explicit grant", false, false, http.StatusForbidden},
		{"strict missing grant", true, false, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := sitePermissionRequest(userID, site.ID)
			grants := map[legacyuuid.UUID]auth.SiteRole{}
			if tc.hasGrant {
				grants[site.ID] = auth.SiteViewer
			}
			r = r.WithContext(context.WithValue(r.Context(), APIClientAuthKey, &database.APIClientAuth{ClientID: legacyuuid.UUID(uuid.NewV7()), UserID: userID, InstanceRole: auth.InstanceOwner, SiteRoles: grants}))
			w := httptest.NewRecorder()
			if tc.strict {
				c.RequireSiteOrInstancePermission(auth.PermSiteManageData, auth.PermInstanceManageImports)(next)(w, r)
			} else {
				c.RequirePermission(auth.PermSiteManageData)(next)(w, r)
			}
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestIntegrationAuthenticationRejectsCookiesAndInvalidOAuth(t *testing.T) {
	c := &Context{}
	for _, header := range []string{"", "Bearer hk_oauth_at_invalid", "Bearer hk_oauth_rt_invalid"} {
		r := httptest.NewRequest(http.MethodGet, "/api/integrations/sites", nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "irrelevant-cookie"})
		r.Header.Set("Authorization", header)
		w := httptest.NewRecorder()
		called := false
		c.RequireIntegrationAuth("sites:read", func(http.ResponseWriter, *http.Request) { called = true })(w, r)
		if w.Code != http.StatusUnauthorized || called {
			t.Fatalf("credential %q: %d, %v", header, w.Code, called)
		}
	}
}

func TestAuthorizeOAuthSiteUsesConsentTeamAndCurrentRole(t *testing.T) {
	store := newSharedTestStore(t)
	defer store.Close()
	userID, err := store.CreateUser(t.Context(), "oauth-site@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(t.Context(), userID, "oauth-site.example.test")
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.GetSiteTenantID(t.Context(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherTeam, err := store.CreateTenant(t.Context(), userID, "selected dashboard team", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveTenantID(t.Context(), userID, otherTeam.ID); err != nil {
		t.Fatal(err)
	}
	grant := &auth.OAuthGrant{ID: uuid.NewV7(), UserID: uuid.UUID(userID), TeamID: uuid.UUID(teamID), SiteIDs: []uuid.UUID{uuid.UUID(site.ID)}, Scopes: []string{"sites:read", "ingest:write"}, Audience: "https://example.test/api"}
	c := &Context{Store: store}
	if err := c.AuthorizeOAuthSite(t.Context(), grant, uuid.UUID(site.ID), grant.Audience, "ingest:write", auth.PermSiteManageData); err != nil {
		t.Fatal(err)
	}
	if err := store.Exec(t.Context(), "UPDATE site_members SET role = 'viewer' WHERE site_id = ? AND user_id = ?", site.ID, userID); err != nil {
		t.Fatal(err)
	}
	if err := c.AuthorizeOAuthSite(t.Context(), grant, uuid.UUID(site.ID), grant.Audience, "ingest:write", auth.PermSiteManageData); err == nil {
		t.Fatal("demoted actor still ingests")
	}
	if err := c.AuthorizeOAuthSite(t.Context(), grant, uuid.UUID(site.ID), "https://example.test/mcp", "sites:read", auth.PermSiteView); err == nil {
		t.Fatal("API grant accepted at MCP audience")
	}
	grant.SiteIDs = []uuid.UUID{uuid.NewV7()}
	if err := c.AuthorizeOAuthSite(t.Context(), grant, uuid.UUID(site.ID), grant.Audience, "sites:read", auth.PermSiteView); err == nil {
		t.Fatal("site outside consent accepted")
	}
}
