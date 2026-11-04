//go:build billing

package cloud

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"hitkeep/database"
	"hitkeep/oauth"
)

func TestVerifySignupResumesOnlyValidBrowserBoundOAuth(t *testing.T) {
	for _, tc := range []struct{ name, plan, path string }{
		{"ordinary", database.CloudPlanFree, "/dashboard"},
		{"valid", database.CloudPlanFree, "/oauth/consent"},
		{"forged", database.CloudPlanFree, "/dashboard"},
		{"other browser", database.CloudPlanFree, "/dashboard"},
		{"expired", database.CloudPlanFree, "/dashboard"},
		{"paid", database.CloudPlanPro, "/signup/verified?billing=monthly&plan=pro"},
		{"outage", database.CloudPlanFree, "/oauth/consent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, store := setupCloudTestHandler(t)
			t.Cleanup(func() { _ = store.Close() })
			ready := true
			provider, err := oauth.New(store, oauth.Options{Issuer: h.ctx.Config.PublicURL, MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("b", 32)), Clients: []oauth.ClientMetadata{{ID: "wordpress", Name: "WordPress", RedirectURIs: []string{"https://wordpress.example/callback"}, AuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"}}}, IsLeader: func() bool { return ready }})
			if err != nil {
				t.Fatal(err)
			}
			h.ctx.OAuthProvider = provider
			verifier := strings.Repeat("v", 64)
			challenge := sha256.Sum256([]byte(verifier))
			query := url.Values{"response_type": {"code"}, "client_id": {"wordpress"}, "redirect_uri": {"https://wordpress.example/callback"}, "resource": {provider.Resource("api")}, "scope": {"sites:read analytics:read"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}, "screen_hint": {"signup"}}
			authorization := httptest.NewRecorder()
			provider.Authorize(authorization, httptest.NewRequest(http.MethodGet, provider.Issuer()+"/oauth/authorize?"+query.Encode(), nil))
			if authorization.Code != http.StatusSeeOther {
				t.Fatalf("authorize: %d %s", authorization.Code, authorization.Body.String())
			}
			token, err := store.CreatePendingSignup(t.Context(), database.PendingSignupEntry{Email: "signup-oauth@example.test", HashedPassword: "unused-hash", TeamName: "WordPress", Jurisdiction: "EU", Locale: "en", PlanCode: tc.plan, BillingInterval: "monthly"})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/cloud/signup/verify?token="+token, nil)
			if tc.name != "ordinary" {
				for _, cookie := range authorization.Result().Cookies() {
					if tc.name == "forged" && cookie.Name == "hk_oauth_pending" {
						cookie.Value = "forged"
					}
					if tc.name == "other browser" && cookie.Name == "hk_oauth_browser" {
						cookie.Value = strings.Repeat("a", 43)
					}
					request.AddCookie(cookie)
				}
			}
			if tc.name == "expired" {
				if err := store.Exec(t.Context(), "UPDATE oauth_pending SET expires_at = CURRENT_TIMESTAMP - INTERVAL 1 HOUR"); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "outage" {
				ready = false
				response := httptest.NewRecorder()
				h.handleVerifySignup()(response, request)
				if response.Code != http.StatusServiceUnavailable {
					t.Fatalf("outage lost continuation: %d %s", response.Code, response.Header().Get("Location"))
				}
				user, err := store.GetUserByEmail(t.Context(), "signup-oauth@example.test")
				if err != nil || user != nil {
					t.Fatalf("outage consumed signup: %v %v", user, err)
				}
				ready = true
			}
			response := httptest.NewRecorder()
			h.handleVerifySignup()(response, request)
			if response.Code != http.StatusFound || response.Header().Get("Location") != h.ctx.Config.PublicURL+tc.path {
				t.Fatalf("redirect: %d %q want %q", response.Code, response.Header().Get("Location"), tc.path)
			}
		})
	}
}
