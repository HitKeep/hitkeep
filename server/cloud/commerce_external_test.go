//go:build billing

package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	legacyuuid "github.com/google/uuid"

	"hitkeep/database"
	"hitkeep/oauth"
)

func TestCommerceExternalOAuthLifecycle(t *testing.T) {
	for _, protocol := range []string{"acp", "ucp"} {
		t.Run(protocol, func(t *testing.T) {
			h, store := setupCloudTestHandler(t)
			t.Cleanup(func() { _ = store.Close() })
			h.ctx.Config.AIEnabled = true
			h.profileCheck = func(context.Context, string) (int, error) { return 0, nil }
			buyer, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{Email: protocol + "-external@example.test", HashedPassword: "hash", TeamName: "External"})
			if err != nil {
				t.Fatal(err)
			}
			provider, err := oauth.New(store, oauth.Options{Issuer: "https://example.test", MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("k", 32)), IsLeader: func() bool { return true }, AgentActionsEnabled: func() bool { return h.ctx.Config.AIEnabled }})
			if err != nil {
				t.Fatal(err)
			}
			h.ctx.OAuthProvider = provider
			fake := h.stripe.(*fakeStripeClient)
			fake.prices = map[string]*stripePriceOutput{"price_pro": {AmountMinor: 1200, Currency: "EUR", Interval: "month", Active: true}}
			issue := func(team legacyuuid.UUID, scopes []string, sites []uuid.UUID) (string, uuid.UUID) {
				t.Helper()
				id := uuid.NewV7()
				entropy := sha256.Sum256([]byte(id.String()))
				token := "hk_oauth_at_" + base64.RawURLEncoding.EncodeToString(entropy[:])
				tx, err := store.BeginOAuth(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback() }()
				now := time.Now().UTC()
				if err := tx.CreateGrant(t.Context(), database.OAuthGrant{ID: id, UserID: uuid.UUID(buyer.UserID), TeamID: uuid.UUID(team), ClientID: "external-test", ClientName: "External test", Audience: provider.Resource("api"), Scopes: scopes, SiteIDs: sites, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
					t.Fatal(err)
				}
				if err := tx.CreateCredential(t.Context(), database.OAuthCredential{Kind: "access", Signature: fmt.Sprintf("%x", sha256.Sum256([]byte(token))), GrantID: id, TeamID: uuid.UUID(team), Payload: []byte(`{}`), ExpiresAt: now.Add(time.Hour)}); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				return token, id
			}
			// Billing consent is team-scoped and must work before the first site exists.
			token, grantID := issue(buyer.TenantID, []string{oauth.ScopeBillingWrite}, nil)
			mux := http.NewServeMux()
			h.registerCommerce(mux)
			base := "/api/cloud/commerce/acp/checkout_sessions"
			body := `{"items":[{"id":"pro:monthly","quantity":1}]}`
			if protocol == "ucp" {
				base = "/api/cloud/commerce/ucp/checkout-sessions"
				body = `{"line_items":[{"item":{"id":"pro:monthly"},"quantity":1}]}`
			}
			call := func(method, path, key, payload, credential string) *httptest.ResponseRecorder {
				t.Helper()
				r := httptest.NewRequest(method, path, strings.NewReader(payload))
				r.Header.Set("API-Version", acpVersion)
				r.Header.Set("Request-Id", "external-request")
				r.Header.Set("UCP-Agent", `profile="https://agent.example/.well-known/ucp"`)
				r.Header.Set("Idempotency-Key", key)
				if credential != "" {
					r.Header.Set("Authorization", "Bearer "+credential)
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				return w
			}
			if w := call("POST", base, "unauth", body, ""); w.Code != 401 {
				t.Fatalf("unauthenticated=%d %s", w.Code, w.Body.String())
			}
			w := call("POST", base, "create", body, token)
			if w.Code != 201 || !strings.Contains(w.Body.String(), `"requires_escalation"`) || !strings.Contains(w.Body.String(), `"continue_url":"https://checkout.stripe.test/session"`) {
				t.Fatalf("external create=%d %s", w.Code, w.Body.String())
			}
			if w := call("POST", base, "create", body, token); w.Code != 201 || fake.checkoutCalls != 1 || w.Header().Get("Idempotent-Replayed") != "true" {
				t.Fatalf("replay=%d creates=%d %s", w.Code, fake.checkoutCalls, w.Body.String())
			}
			if w := call("POST", base+"/cs_test/complete", "complete", `{}`, token); w.Code != 200 || !strings.Contains(w.Body.String(), `"requires_escalation"`) {
				t.Fatalf("machine completion=%d %s", w.Code, w.Body.String())
			}
			other, err := store.CreateTenant(t.Context(), buyer.UserID, "Other", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.SetActiveTenantID(t.Context(), buyer.UserID, other.ID); err != nil {
				t.Fatal(err)
			}
			if w := call("GET", base+"/cs_test", "", "", token); w.Code != 200 {
				t.Fatalf("grant team changed with dashboard=%d %s", w.Code, w.Body.String())
			}
			wrongTeam, _ := issue(other.ID, []string{oauth.ScopeBillingWrite}, nil)
			if w := call("GET", base+"/cs_test", "", "", wrongTeam); w.Code != 404 {
				t.Fatalf("cross-team=%d %s", w.Code, w.Body.String())
			}
			if err := store.SetActiveTenantID(t.Context(), buyer.UserID, buyer.TenantID); err != nil {
				t.Fatal(err)
			}
			site, err := store.CreateSite(t.Context(), buyer.UserID, protocol+"-external.example.test")
			if err != nil {
				t.Fatal(err)
			}
			readOnly, _ := issue(buyer.TenantID, []string{oauth.ScopeAnalyticsRead}, []uuid.UUID{uuid.UUID(site.ID)})
			if w := call("POST", base, "read-only", body, readOnly); w.Code != 403 {
				t.Fatalf("read-only=%d %s", w.Code, w.Body.String())
			}
			h.ctx.Config.AIEnabled = false
			if w := call("GET", base+"/cs_test", "", "", token); w.Code != 404 {
				t.Fatalf("AI off=%d %s", w.Code, w.Body.String())
			}
			h.ctx.Config.AIEnabled = true
			update := "POST"
			if protocol == "ucp" {
				update = "PUT"
			}
			if w := call(update, base+"/cs_test", "update", body, token); w.Code != 200 || !strings.Contains(w.Body.String(), `"requires_escalation"`) {
				t.Fatalf("external update=%d %s", w.Code, w.Body.String())
			}
			if w := call("POST", base+"/cs_test/cancel", "cancel", `{}`, token); w.Code != 200 || !strings.Contains(w.Body.String(), `"canceled"`) {
				t.Fatalf("external cancel=%d %s", w.Code, w.Body.String())
			}
			if err := store.AddTeamMember(t.Context(), buyer.TenantID, buyer.UserID, database.TenantRoleMember, buyer.UserID); err != nil {
				t.Fatal(err)
			}
			if w := call("POST", base, "create", body, token); w.Code != 403 {
				t.Fatalf("lost billing role replay=%d %s", w.Code, w.Body.String())
			}
			if err := store.AddTeamMember(t.Context(), buyer.TenantID, buyer.UserID, database.TenantRoleOwner, buyer.UserID); err != nil {
				t.Fatal(err)
			}
			if err := store.RevokeOAuthGrant(t.Context(), uuid.UUID(buyer.UserID), grantID); err != nil {
				t.Fatal(err)
			}
			if w := call("POST", base, "create", body, token); w.Code != 401 {
				t.Fatalf("revoked replay=%d %s", w.Code, w.Body.String())
			}
			if fake.checkoutCalls != 1 {
				t.Fatalf("unauthorized calls created %d sessions", fake.checkoutCalls)
			}
		})
	}
}
