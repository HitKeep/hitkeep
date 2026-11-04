package shared

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	legacyuuid "github.com/google/uuid"
	"hitkeep/auth"
	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/oauth"
)

func TestEmbedReadOnlyConsentAndRevocation(t *testing.T) {
	store := newSharedTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	user, err := store.CreateUser(t.Context(), "embed@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(t.Context(), user, "embed.example")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.GetSiteTenantID(t.Context(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := oauth.New(store, oauth.Options{Issuer: "https://example.test", MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("k", 32)), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	c := &Context{Store: store, OAuthProvider: p, Config: &config.Config{PublicURL: p.Issuer(), JWTSecret: "test-embed-signing-secret"}}
	grant := database.OAuthGrant{ID: uuid.NewV7(), UserID: uuid.UUID(user), TeamID: uuid.UUID(team), ClientID: "https://wordpress.example/client.json", ClientName: "WordPress", Audience: p.Resource("api"), Scopes: []string{oauth.ScopeAnalyticsRead}, SiteIDs: []uuid.UUID{uuid.UUID(site.ID)}, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	tx, err := store.BeginOAuth(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.CreateGrant(t.Context(), grant); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateEmbedToken(c.Config.JWTSecret, c.Config.PublicURL, grant.ID, uuid.UUID(site.ID), "https://wordpress.example", time.Now().Add(auth.EmbedTokenLifetime))
	if err != nil {
		t.Fatal(err)
	}
	next := func(w http.ResponseWriter, r *http.Request) {
		if GetUserIDFromContext(r) != user {
			t.Error("wrong actor")
		}
		w.WriteHeader(http.StatusNoContent)
	}
	for _, tc := range []struct {
		name, method, path, origin string
		site                       uuid.UUID
		want                       int
	}{
		{"analytics", http.MethodGet, "/stats", "https://wordpress.example", uuid.UUID(site.ID), 204},
		{"wrong parent", http.MethodGet, "/stats", "https://attacker.example", uuid.UUID(site.ID), 401},
		{"cross site", http.MethodGet, "/stats", "https://wordpress.example", uuid.NewV7(), 401},
		{"write", http.MethodPost, "/stats/reset", "https://wordpress.example", uuid.UUID(site.ID), 403},
		{"raw hits", http.MethodGet, "/hits", "https://wordpress.example", uuid.UUID(site.ID), 403},
		{"export", http.MethodGet, "/hits/export", "https://wordpress.example", uuid.UUID(site.ID), 403},
		{"settings", http.MethodGet, "/members", "https://wordpress.example", uuid.UUID(site.ID), 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/api/sites/"+tc.site.String()+tc.path, nil)
			r.SetPathValue("id", tc.site.String())
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("X-HitKeep-Embed-Parent", tc.origin)
			w := httptest.NewRecorder()
			c.Handler(HandlerConfig{SitePerm: auth.PermSiteView}, next)(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
	for _, tc := range []struct{ name, change, restore string }{
		{"unselected site", "DELETE FROM oauth_grant_sites WHERE oauth_grant_id = ?", "INSERT INTO oauth_grant_sites (oauth_grant_id,tenant_id,site_id) VALUES (?, ?, ?)"},
		{"scope revoked", "UPDATE oauth_grants SET scopes = '[]' WHERE id = ?", "UPDATE oauth_grants SET scopes = '[\"analytics:read\"]' WHERE id = ?"},
		{"grant revoked", "UPDATE oauth_grants SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", "UPDATE oauth_grants SET revoked_at = NULL WHERE id = ?"},
		{"grant expired", "UPDATE oauth_grants SET expires_at = TIMESTAMP '2000-01-01' WHERE id = ?", "UPDATE oauth_grants SET expires_at = CURRENT_TIMESTAMP + INTERVAL 1 HOUR WHERE id = ?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.Exec(t.Context(), tc.change, grant.ID.String()); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/stats", nil)
			r.SetPathValue("id", site.ID.String())
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("X-HitKeep-Embed-Parent", "https://wordpress.example")
			w := httptest.NewRecorder()
			c.Handler(HandlerConfig{SitePerm: auth.PermSiteView}, next)(w, r)
			if w.Code != 401 {
				t.Fatalf("revoked consent accepted: %d %s", w.Code, w.Body.String())
			}
			if tc.name == "unselected site" {
				err = store.Exec(t.Context(), tc.restore, grant.ID.String(), grant.TeamID.String(), site.ID.String())
			} else {
				err = store.Exec(t.Context(), tc.restore, grant.ID.String())
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := store.Exec(t.Context(), "DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?", team, user); err != nil {
		t.Fatal(err)
	}
	// Consent cannot restore a removed current team membership.
	r := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/stats", nil)
	r.SetPathValue("id", site.ID.String())
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-HitKeep-Embed-Parent", "https://wordpress.example")
	w := httptest.NewRecorder()
	c.Handler(HandlerConfig{SitePerm: auth.PermSiteView}, next)(w, r)
	if w.Code != 401 {
		t.Fatalf("removed actor accepted: %d %s", w.Code, w.Body.String())
	}
}

func TestEmbedNeverFallsBackToSession(t *testing.T) {
	c := &Context{Config: &config.Config{PublicURL: "https://example.test", JWTSecret: "cookie-test-secret"}}
	cookie, err := auth.GenerateToken(c.Config.JWTSecret, c.Config.PublicURL, legacyuuid.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/user", "/api/sites/" + uuid.NewV7().String() + "/stats"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer hk_embed_invalid")
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		w := httptest.NewRecorder()
		called := false
		c.RequireAuth(true, func(http.ResponseWriter, *http.Request) { called = true })(w, r)
		if called || w.Code < 400 {
			t.Fatalf("embed credential fell back to cookie: %d", w.Code)
		}
	}
}
