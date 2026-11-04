package oauth

import (
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

func TestAgentRefreshUsesCurrentApplicationMembership(t *testing.T) {
	store := database.NewStore(filepath.Join(t.TempDir(), "agent.db"))
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(t.Context(), "claimed@example.test", "unused-hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(t.Context(), user, "claimed.example")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.GetSiteTenantID(t.Context(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := provider.New(store, provider.Options{Issuer: "https://hitkeep.example", MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("a", 32)), IsLeader: func() bool { return true }, AgentActionsEnabled: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{ctx: &shared.Context{Store: store, Config: &config.Config{PublicURL: p.Issuer(), AIEnabled: true}, OAuthProvider: p}}
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	r := httptest.NewRequest("POST", "/agent/identity", strings.NewReader(`{"type":"service_auth","login_hint":"claimed@example.test"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	p.AgentIdentity(w, r)
	var registration struct {
		ClaimToken string `json:"claim_token"`
		Claim      struct {
			Code string `json:"user_code"`
			URI  string `json:"verification_uri"`
		} `json:"claim"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &registration); err != nil || w.Code != 201 {
		t.Fatalf("registration: %d %s %v", w.Code, w.Body.String(), err)
	}
	browser := httptest.NewRecorder()
	p.StartAgentClaim(browser, httptest.NewRequest("GET", registration.Claim.URI, nil))
	if browser.Code != 303 {
		t.Fatalf("claim: %d %s", browser.Code, browser.Body.String())
	}
	request := httptest.NewRequest("POST", "/api/oauth/consent", nil)
	for _, cookie := range browser.Result().Cookies() {
		request.AddCookie(cookie)
	}
	consentResponse := httptest.NewRecorder()
	h.consent(consentResponse, request, uuid.UUID(user))
	var consent api.OAuthConsent
	if err := json.Unmarshal(consentResponse.Body.Bytes(), &consent); err != nil || consentResponse.Code != 200 || len(consent.Teams) != 1 || len(consent.Teams[0].Sites) != 1 {
		t.Fatalf("consent: %d %s %v", consentResponse.Code, consentResponse.Body.String(), err)
	}
	form := url.Values{"decision": {"approve"}, "csrf_token": {consent.CSRFToken}, "team_id": {team.String()}, "site_ids": {site.ID.String()}, "claim_code": {registration.Claim.Code}}
	approve := httptest.NewRequest("POST", "/api/oauth/consent", strings.NewReader(form.Encode()))
	approve.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range browser.Result().Cookies() {
		approve.AddCookie(cookie)
	}
	approved := httptest.NewRecorder()
	h.completeConsent(approved, approve, uuid.UUID(user))
	if approved.Code != 303 || !strings.Contains(approved.Header().Get("Location"), "claim=complete") {
		t.Fatalf("approval: %d %s", approved.Code, approved.Header().Get("Location"))
	}
	exchange := func(form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	tokens := exchange(url.Values{"grant_type": {"urn:workos:agent-auth:grant-type:claim"}, "claim_token": {registration.ClaimToken}})
	var issued struct {
		Assertion string `json:"identity_assertion"`
		Access    string `json:"access_token"`
	}
	if err := json.Unmarshal(tokens.Body.Bytes(), &issued); err != nil || tokens.Code != 200 || issued.Assertion == "" || issued.Access == "" {
		t.Fatalf("issued: %d %s %v", tokens.Code, tokens.Body.String(), err)
	}
	refresh := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {issued.Assertion}, "resource": {p.Resource("api")}}
	if w := exchange(refresh); w.Code != 200 {
		t.Fatalf("authorized refresh: %d %s", w.Code, w.Body.String())
	}
	if err := store.Exec(t.Context(), "DELETE FROM tenant_members WHERE tenant_id=? AND user_id=?", team, user); err != nil {
		t.Fatal(err)
	}
	denied := exchange(refresh)
	if denied.Code != 400 || strings.Contains(denied.Body.String(), "access_token") {
		t.Fatalf("removed membership refreshed: %d %s", denied.Code, denied.Body.String())
	}
}
