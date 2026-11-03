package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"hitkeep/api"
	"hitkeep/config"
	"hitkeep/database"
	provider "hitkeep/oauth"
	"hitkeep/server/shared"
)

func TestOAuthFirstSiteSetupPreservesExplicitConsent(t *testing.T) {
	store := database.NewStore(filepath.Join(t.TempDir(), "onboarding.db"))
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(t.Context(), "new-wordpress@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.GetActiveTenantID(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	p, err := provider.New(store, provider.Options{Issuer: "https://hitkeep.example", MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("k", 32)), IsLeader: func() bool { return true }, Clients: []provider.ClientMetadata{{ID: "wp-test", Name: "WordPress", RedirectURIs: []string{"https://shop.example/callback"}, AuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"}}}})
	if err != nil {
		t.Fatal(err)
	}
	challenge := sha256.Sum256([]byte(strings.Repeat("v", 64)))
	query := url.Values{"client_id": {"wp-test"}, "response_type": {"code"}, "resource": {p.Resource("api")}, "scope": {"sites:read analytics:read ingest:write"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}, "site_domain": {"shop.example"}}
	start := httptest.NewRecorder()
	p.Authorize(start, httptest.NewRequest(http.MethodGet, p.Issuer()+"/oauth/authorize?"+query.Encode(), nil))
	if start.Code != 303 {
		t.Fatalf("authorize: %d %s", start.Code, start.Body.String())
	}
	request := func(body string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, p.Issuer()+"/api/oauth/sites", strings.NewReader(body))
		for _, cookie := range start.Result().Cookies() {
			r.AddCookie(cookie)
		}
		return r.WithContext(context.WithValue(r.Context(), shared.UserIDKey, user))
	}
	h := handler{ctx: &shared.Context{Store: store, Config: &config.Config{PublicURL: p.Issuer()}, OAuthProvider: p}}
	info, err := p.Consent(t.Context(), request(""), uuid.UUID(user))
	if err != nil {
		t.Fatal(err)
	}
	choices := httptest.NewRecorder()
	h.consent(choices, request(""), uuid.UUID(user))
	var consent api.OAuthConsent
	if err := json.Unmarshal(choices.Body.Bytes(), &consent); err != nil {
		t.Fatal(err)
	}
	if len(consent.Teams) != 1 || !consent.Teams[0].CanCreateSite || len(consent.Teams[0].Sites) != 0 {
		t.Fatalf("new account dead end: %s", choices.Body.String())
	}
	for _, tc := range []struct {
		name, csrf, team string
		want             int
	}{
		{"csrf", "wrong", team.String(), 400},
		{"other team", info.CSRFToken, uuid.NewV7().String(), 403},
		{"create", info.CSRFToken, team.String(), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"domain": "shop.example", "csrf_token": tc.csrf, "team_id": tc.team})
			w := httptest.NewRecorder()
			h.createSite(w, request(string(body)), uuid.UUID(user))
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
	if pending, err := p.HasContinuation(t.Context(), request("")); err != nil || !pending {
		t.Fatalf("creating site lost pending consent: %v %v", pending, err)
	}
	grants, err := store.ListOAuthGrants(t.Context(), uuid.UUID(user))
	if err != nil || len(grants) != 0 {
		t.Fatalf("site creation granted consent implicitly: %v %+v", err, grants)
	}
}
