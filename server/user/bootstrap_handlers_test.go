package user

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	json "hitkeep/jsonapi"
	"hitkeep/oauth"
	"hitkeep/server/shared"
)

func TestHandleGetUserBootstrap(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.MCPEnabled = true
	h.ctx.OAuthProvider = bootstrapOAuthProvider(t, h, true)

	if _, err := store.CreateSite(context.Background(), userID, "bootstrap.example.com"); err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.UpsertUserPreferences(context.Background(), userID, api.UserPreferences{DefaultLocale: "de"}); err != nil {
		t.Fatalf("upsert preferences: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://spoofed.example/api/user/bootstrap", nil)
	req.Header.Set("X-Forwarded-Host", "spoofed.example")
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	req = req.WithContext(context.WithValue(req.Context(), shared.AuthSessionKey, shared.AuthSessionContext{
		ExpiresAt: time.Now().Add(30 * time.Minute).UTC(),
		IssuedAt:  time.Now().UTC(),
	}))
	w := httptest.NewRecorder()

	h.handleGetUserBootstrap().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.UserBootstrap
	if err := json.UnmarshalRead(w.Body, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Session.ExpiresAt.IsZero() {
		t.Fatalf("expected session details")
	}
	if resp.Profile.ID != userID || resp.Profile.Email == "" {
		t.Fatalf("expected profile for user %s, got %+v", userID, resp.Profile)
	}
	if resp.Preferences.DefaultLocale != "de" {
		t.Fatalf("expected locale de, got %q", resp.Preferences.DefaultLocale)
	}
	if resp.Teams.ActiveTeamID == uuid.Nil || len(resp.Teams.Teams) == 0 {
		t.Fatalf("expected team context, got %+v", resp.Teams)
	}
	if resp.Permissions.InstanceRole == "" {
		t.Fatalf("expected permission context")
	}
	if resp.Permissions.ActiveTeamID == nil || *resp.Permissions.ActiveTeamID == uuid.Nil || resp.Permissions.ActiveTeamRole != "owner" {
		t.Fatalf("expected active team access context, got %+v", resp.Permissions)
	}
	if len(resp.Permissions.ActiveTeamCapabilities) == 0 || len(resp.Permissions.InstanceCapabilities) == 0 {
		t.Fatalf("expected derived access capabilities, got %+v", resp.Permissions)
	}
	if len(resp.Sites) != 1 || resp.Sites[0].Domain != "bootstrap.example.com" {
		t.Fatalf("expected bootstrap site, got %+v", resp.Sites)
	}
	if !resp.Assistants.Enabled || resp.Assistants.Endpoint != "https://analytics.example/prefix/assistant/mcp" {
		t.Fatalf("expected canonical prefixed endpoint independent of Host, got %+v", resp.Assistants)
	}
	if resp.Status.NeedsSetup {
		t.Fatalf("expected setup to be complete")
	}

	h.ctx.Config.MCPEnabled = false
	w = httptest.NewRecorder()
	h.handleGetUserBootstrap().ServeHTTP(w, req)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `"endpoint"`) {
		t.Fatalf("disabled MCP must omit endpoint: status=%d body=%s", w.Code, w.Body.String())
	}
	var disabled api.UserBootstrap
	if err := json.UnmarshalRead(w.Body, &disabled); err != nil {
		t.Fatal(err)
	}
	if disabled.Assistants.Enabled {
		t.Fatal("disabled MCP must not advertise assistant availability")
	}
}

func TestHandleGetUserBootstrapRequiresSession(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/bootstrap", nil), userID)
	w := httptest.NewRecorder()

	h.handleGetUserBootstrap().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func bootstrapOAuthProvider(t *testing.T, h *handler, enabled bool) *oauth.Provider {
	t.Helper()
	provider, err := oauth.New(h.ctx.Store, oauth.Options{
		Issuer: "https://analytics.example/prefix", MCPPath: "/assistant/mcp", MCPEnabled: enabled,
		BrowserSigningKey: []byte(strings.Repeat("b", 32)), IsLeader: func() bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type bootstrapClusterState bool

func (c bootstrapClusterState) IsLeader() bool        { return bool(c) }
func (c bootstrapClusterState) GetLeaderAddr() string { return "private-node.example:8080" }

func TestAssistantConnectionResponse(t *testing.T) {
	h, store, _ := setupUserSecurityTestEnv(t)
	defer store.Close()
	provider := bootstrapOAuthProvider(t, h, true)
	disabledProvider := bootstrapOAuthProvider(t, h, false)
	conf := h.ctx.Config

	for _, tc := range []struct {
		name         string
		configure    func()
		wantEnabled  bool
		wantEndpoint bool
	}{
		{name: "standalone", configure: func() {}, wantEnabled: true, wantEndpoint: true},
		{name: "leader", configure: func() { h.ctx.Cluster = bootstrapClusterState(true) }, wantEnabled: true, wantEndpoint: true},
		{name: "follower", configure: func() { h.ctx.Cluster = bootstrapClusterState(false) }},
		{name: "disabled", configure: func() { h.ctx.Config.MCPEnabled = false }},
		{name: "missing config", configure: func() { h.ctx.Config = nil }},
		{name: "missing store", configure: func() { h.ctx.Store = nil }},
		{name: "legacy API token route without OAuth", configure: func() { h.ctx.OAuthProvider = nil }, wantEnabled: true},
		{name: "disabled provider resource", configure: func() { h.ctx.OAuthProvider = disabledProvider }, wantEnabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h.ctx.Config = conf
			h.ctx.Config.MCPEnabled = true
			h.ctx.Store = store
			h.ctx.Cluster = nil
			h.ctx.OAuthProvider = provider
			tc.configure()
			got := h.assistantConnectionResponse()
			if got.Enabled != tc.wantEnabled {
				t.Fatalf("enabled=%v, want %v", got.Enabled, tc.wantEnabled)
			}
			wantEndpoint := ""
			if tc.wantEndpoint {
				wantEndpoint = "https://analytics.example/prefix/assistant/mcp"
			}
			if got.Endpoint != wantEndpoint {
				t.Fatalf("endpoint=%q, want %q", got.Endpoint, wantEndpoint)
			}
		})
	}
}

func TestHandleGetUserBootstrapRequiresUserAndStore(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	for _, tc := range []struct {
		name string
		user bool
		want int
	}{
		{name: "anonymous", want: http.StatusUnauthorized},
		{name: "unavailable store", user: true, want: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/user/bootstrap", nil)
			if tc.user {
				req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
			}
			h.ctx.Store = nil
			w := httptest.NewRecorder()
			h.handleGetUserBootstrap().ServeHTTP(w, req)
			if w.Code != tc.want || strings.Contains(w.Body.String(), "endpoint") {
				t.Fatalf("status=%d, want %d, body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
