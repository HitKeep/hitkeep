package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/config"
	"hitkeep/oauth"
	"hitkeep/server/annotations"
	serveroauth "hitkeep/server/oauth"
	"hitkeep/server/shared"
	"hitkeep/server/sites"
)

func TestOffsiteOAuthActionsConsentPermissionsAndRevocation(t *testing.T) {
	store := testServerStore(t)
	t.Cleanup(func() { _ = store.Close() })
	user, err := store.CreateUser(t.Context(), "offsite@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(t.Context(), user, "offsite.example.test")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.GetSiteTenantID(t.Context(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	conf := &config.Config{PublicURL: "https://example.test", JWTSecret: strings.Repeat("s", 32), AIEnabled: true}
	p, err := oauth.New(store, oauth.Options{Issuer: conf.PublicURL, MCPPath: "/mcp", MCPEnabled: true, BrowserSigningKey: []byte(conf.JWTSecret), IsLeader: func() bool { return true }, AgentActionsEnabled: func() bool { return conf.AIEnabled }, Clients: []oauth.ClientMetadata{{ID: "offsite", Name: "Offsite assistant", RedirectURIs: []string{"https://assistant.example/callback"}, AuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"}}}})
	if err != nil {
		t.Fatal(err)
	}
	c := &shared.Context{Store: store, Config: conf, OAuthProvider: p}
	mux := http.NewServeMux()
	serveroauth.Register(mux, c)
	annotations.Register(mux, c)
	sites.Register(mux, c)
	mux.HandleFunc("POST /api/test/billing", c.Handler(shared.HandlerConfig{RequireAuth: true, OAuthScope: oauth.ScopeBillingWrite, ActiveTeamCap: auth.CapTeamManageSettings}, func(w http.ResponseWriter, r *http.Request) {
		id, err := c.ResolveRequestTeamID(r)
		if err != nil {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(id.String()))
	}))
	cookie, err := auth.GenerateToken(conf.JWTSecret, conf.PublicURL, user)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	approve := func(scope string, selectedSites []string) (string, string) {
		t.Helper()
		verifier := strings.Repeat("v", 64)
		digest := sha256.Sum256([]byte(verifier))
		form := url.Values{"response_type": {"code"}, "client_id": {"offsite"}, "resource": {p.Resource("api")}, "scope": {scope}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}}
		w := request("GET", "/oauth/authorize?"+form.Encode(), "", "")
		if w.Code != 303 || w.Header().Get("Location") != p.Issuer()+"/oauth/consent" {
			t.Fatalf("authorize: %d %s", w.Code, w.Body.String())
		}
		browser := w.Result().Cookies()
		consentRequest := func(method, body string) *http.Request {
			r := httptest.NewRequest(method, "/api/oauth/consent", strings.NewReader(body))
			for _, item := range browser {
				r.AddCookie(item)
			}
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return r
		}
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, consentRequest("GET", ""))
		var choices api.OAuthConsent
		if err := json.Unmarshal(w.Body.Bytes(), &choices); err != nil || w.Code != 200 {
			t.Fatalf("consent choices: %d %s %v", w.Code, w.Body.String(), err)
		}
		found := false
		for _, choice := range choices.Teams {
			if uuid.UUID(choice.ID) == team {
				found = true
			}
		}
		if !found {
			t.Fatalf("consent team missing: %s", w.Body.String())
		}
		decision := url.Values{"decision": {"approve"}, "csrf_token": {choices.CSRFToken}, "team_id": {team.String()}}
		for _, id := range selectedSites {
			decision.Add("site_ids", id)
		}
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, consentRequest("POST", decision.Encode()))
		callback, err := url.Parse(w.Header().Get("Location"))
		if err != nil || w.Code != 303 || callback.Query().Get("code") == "" {
			t.Fatalf("consent: %d %s %s", w.Code, w.Header().Get("Location"), w.Body.String())
		}
		exchange := url.Values{"grant_type": {"authorization_code"}, "client_id": {"offsite"}, "resource": {p.Resource("api")}, "code": {callback.Query().Get("code")}, "code_verifier": {verifier}}
		r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(exchange.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		var tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &tokens); err != nil || w.Code != 200 || tokens.AccessToken == "" {
			t.Fatalf("exchange: %d %s %v", w.Code, w.Body.String(), err)
		}
		return tokens.AccessToken, tokens.RefreshToken
	}
	write, refresh := approve("sites:read analytics:read annotations:write", []string{site.ID.String()})
	readonly, _ := approve("sites:read analytics:read", []string{site.ID.String()})
	path := "/api/sites/" + site.ID.String() + "/annotations"
	body := `{"body":"Offsite note","starts_at":"2026-10-01T12:00:00Z"}`
	if w := request("POST", path, readonly, body); w.Code != 403 {
		t.Fatalf("readonly wrote: %d %s", w.Code, w.Body.String())
	}
	if w := request("POST", path, write, body); w.Code != 201 {
		t.Fatalf("write: %d %s", w.Code, w.Body.String())
	}
	if w := request("GET", path+"?from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z", write, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Offsite note") {
		t.Fatalf("read: %d %s", w.Code, w.Body.String())
	}
	if w := request("GET", "/api/sites", write, ""); w.Code != 200 || !strings.Contains(w.Body.String(), site.Domain) {
		t.Fatalf("sites: %d %s", w.Code, w.Body.String())
	}
	foreign, err := store.CreateSite(t.Context(), user, "outside-consent.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if w := request("POST", "/api/sites/"+foreign.ID.String()+"/annotations", write, body); w.Code != 403 {
		t.Fatalf("foreign site: %d", w.Code)
	}
	if w := request("GET", "/api/sites", write, ""); strings.Contains(w.Body.String(), foreign.Domain) {
		t.Fatal("site listing leaked unconsented site")
	}
	for _, forbidden := range []struct{ method, path string }{{"GET", "/api/sites/" + site.ID.String() + "/hits/export"}, {"DELETE", "/api/sites/" + site.ID.String()}, {"GET", "/api/user/oauth-grants"}} {
		if w := request(forbidden.method, forbidden.path, write, ""); w.Code != 401 {
			t.Fatalf("forbidden route %s: %d", forbidden.path, w.Code)
		}
	}
	conf.AIEnabled = false
	if w := request("POST", path, write, body); w.Code != 404 {
		t.Fatalf("AI-off write: %d", w.Code)
	}
	conf.AIEnabled = true
	if err := store.Exec(t.Context(), "UPDATE site_members SET role='viewer' WHERE site_id=? AND user_id=?", site.ID, user); err != nil {
		t.Fatal(err)
	}
	if w := request("POST", path, write, body); w.Code != 403 {
		t.Fatalf("demoted wrote: %d", w.Code)
	}
	if err := store.Exec(t.Context(), "UPDATE site_members SET role='owner' WHERE site_id=? AND user_id=?", site.ID, user); err != nil {
		t.Fatal(err)
	}
	grant, err := p.Authenticate(t.Context(), write, p.Resource("api"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Authenticate(t.Context(), write, p.Resource("mcp")); err == nil {
		t.Fatal("API token accepted at MCP resource")
	}
	if err := store.RevokeOAuthGrant(t.Context(), grant.UserID, grant.ID); err != nil {
		t.Fatal(err)
	}
	if w := request("POST", path, write, body); w.Code != 401 {
		t.Fatalf("revoked wrote: %d", w.Code)
	}
	refreshForm := url.Values{"grant_type": {"refresh_token"}, "client_id": {"offsite"}, "resource": {p.Resource("api")}, "refresh_token": {refresh}}
	r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(refreshForm.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("revoked refreshed: %d", w.Code)
	}
	billing, _ := approve("billing:write", nil)
	siteWriter, _ := approve("sites:write", []string{site.ID.String()})
	other, err := store.CreateTenant(t.Context(), user, "Unrelated active team", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveTenantID(t.Context(), user, other.ID); err != nil {
		t.Fatal(err)
	}
	retentionPath := "/api/sites/" + site.ID.String() + "/retention"
	if w := request("PUT", retentionPath, siteWriter, `{"days":30}`); w.Code != 200 {
		t.Fatalf("consent-team retention after dashboard switch: %d %s", w.Code, w.Body.String())
	}
	retained, err := store.GetSiteByID(t.Context(), site.ID)
	if err != nil || retained.DataRetentionDays != 30 {
		t.Fatalf("retention not persisted: %v %v", retained, err)
	}
	if w := request("PUT", "/api/sites/"+foreign.ID.String()+"/retention", siteWriter, `{"days":30}`); w.Code != 403 {
		t.Fatalf("unconsented retention: %d", w.Code)
	}
	if err := store.Exec(t.Context(), "UPDATE site_members SET role='viewer' WHERE site_id=? AND user_id=?", site.ID, user); err != nil {
		t.Fatal(err)
	}
	if w := request("PUT", retentionPath, siteWriter, `{"days":60}`); w.Code != 403 {
		t.Fatalf("demoted retention: %d", w.Code)
	}
	if err := store.Exec(t.Context(), "UPDATE site_members SET role='owner' WHERE site_id=? AND user_id=?", site.ID, user); err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest("POST", "/api/sites", strings.NewReader(`{"domain":"stale-create.example.test"}`))
	create.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	create.Header.Set("X-HitKeep-Agent", "webmcp")
	create.Header.Set("X-HitKeep-Agent-Team", team.String())
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, create)
	if created.Code != 409 {
		t.Fatalf("stale team created site: %d %s", created.Code, created.Body.String())
	}
	listing := httptest.NewRequest("GET", "/api/sites", nil)
	listing.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	listing.Header.Set("X-HitKeep-Agent", "webmcp")
	listing.Header.Set("X-HitKeep-Agent-Team", team.String())
	listed := httptest.NewRecorder()
	mux.ServeHTTP(listed, listing)
	if listed.Code != 409 {
		t.Fatalf("stale team listed other sites: %d %s", listed.Code, listed.Body.String())
	}
	if w := request("POST", "/api/test/billing", billing, ""); w.Code != 200 || w.Body.String() != team.String() {
		t.Fatalf("billing binding: %d %s", w.Code, w.Body.String())
	}
	if err := store.Exec(t.Context(), "UPDATE tenant_members SET role='member' WHERE tenant_id=? AND user_id=?", team, user); err != nil {
		t.Fatal(err)
	}
	if w := request("POST", "/api/test/billing", billing, ""); w.Code != 403 {
		t.Fatalf("demoted billing: %d %s", w.Code, w.Body.String())
	}
}
