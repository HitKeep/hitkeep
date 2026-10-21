package oauth

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/auth"
	"hitkeep/config"
	"hitkeep/database"
	provider "hitkeep/oauth"
	"hitkeep/server/shared"
	"hitkeep/testutil/testdb"
)

func TestOAuthClientRoutesEnforceHumanTeamAdministration(t *testing.T) {
	store := testdb.Shared(t)
	ctx := t.Context()
	owner, err := store.CreateUser(ctx, "oauth-admin@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.GetActiveTenantID(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	member, err := store.CreateUser(ctx, "oauth-member@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	memberTeam, err := store.CreateTenant(ctx, member, "Foreign OAuth team", "")
	if err != nil {
		t.Fatal(err)
	}
	if memberTeam.ID == team {
		t.Fatal("foreign-team fixture must use a distinct team")
	}
	admin, err := store.CreateUser(ctx, "oauth-team-admin@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTeamMember(ctx, team, member, database.TenantRoleMember, owner); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTeamMember(ctx, team, admin, database.TenantRoleAdmin, owner); err != nil {
		t.Fatal(err)
	}
	conf := &config.Config{JWTSecret: strings.Repeat("k", 32), PublicURL: "https://hitkeep.example"}
	p, err := provider.New(store, provider.Options{Issuer: conf.PublicURL, MCPPath: "/mcp", BrowserSigningKey: []byte(conf.JWTSecret), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, &shared.Context{Store: store, Config: conf, OAuthProvider: p})
	ownerToken, _, err := auth.GenerateTokenWithDuration(conf.JWTSecret, conf.PublicURL, owner, conf.AuthSessionDuration())
	if err != nil {
		t.Fatal(err)
	}
	memberToken, _, err := auth.GenerateTokenWithDuration(conf.JWTSecret, conf.PublicURL, member, conf.AuthSessionDuration())
	if err != nil {
		t.Fatal(err)
	}
	adminToken, _, err := auth.GenerateTokenWithDuration(conf.JWTSecret, conf.PublicURL, admin, conf.AuthSessionDuration())
	if err != nil {
		t.Fatal(err)
	}
	url := "/api/user/teams/" + team.String() + "/oauth-clients"
	serve := func(method, path, token, bearer, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	body := `{"client_name":"Reporting app","redirect_uris":["https://reports.example/callback"]}`
	for _, tc := range []struct {
		token, bearer string
		status        int
	}{{"", "", 401}, {memberToken, "", 403}, {ownerToken, "some-api-token", 401}} {
		if w := serve("POST", url, tc.token, tc.bearer, body); w.Code != tc.status {
			t.Fatalf("authorization status %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
	created := serve("POST", url, ownerToken, "", body)
	if created.Code != 201 {
		t.Fatalf("register: %d %s", created.Code, created.Body.String())
	}
	var client provider.ClientRegistration
	if err := json.Unmarshal(created.Body.Bytes(), &client); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(created.Body.String(), "secret") || client.ID == "" {
		t.Fatal("unsafe registration response")
	}
	if w := serve("GET", url, adminToken, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), client.ID) {
		t.Fatalf("admin list: %d %s", w.Code, w.Body.String())
	}
	if w := serve("POST", url, ownerToken, "", `{"client_name":"Bad","redirect_uris":["http://example.com"],"client_secret":"injected"}`); w.Code != 400 {
		t.Fatalf("unknown field accepted: %d", w.Code)
	}
	foreignURL := "/api/user/teams/" + memberTeam.ID.String() + "/oauth-clients/" + client.ID
	if w := serve("DELETE", foreignURL, memberToken, "", ""); w.Code != 404 {
		t.Fatalf("foreign client disclosed or revoked: %d %s", w.Code, w.Body.String())
	}
	if w := serve("PUT", url+"/"+client.ID, memberToken, "", body); w.Code != 403 {
		t.Fatalf("member edited client: %d", w.Code)
	}
	if w := serve("DELETE", url+"/"+client.ID, memberToken, "", ""); w.Code != 403 {
		t.Fatalf("member revoked client: %d", w.Code)
	}

	if w := serve("PUT", url+"/"+client.ID, adminToken, "", body); w.Code != 200 {
		t.Fatalf("edit: %d %s", w.Code, w.Body.String())
	}
	if w := serve("DELETE", url+"/"+client.ID, adminToken, "", ""); w.Code != 204 {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
}
