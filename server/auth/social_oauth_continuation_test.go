package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"hitkeep/database"
	"hitkeep/oauth"
	"hitkeep/server/shared"
	"hitkeep/socialauth"
)

func socialOAuthCookies(t *testing.T, h *handler, store *database.Store, ready *bool) []*http.Cookie {
	t.Helper()
	h.ctx.Config.PublicURL = "https://auth.example"
	p, err := oauth.New(store, oauth.Options{Issuer: h.ctx.Config.PublicURL, MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("b", 32)), Clients: []oauth.ClientMetadata{{ID: "wordpress", Name: "WordPress", RedirectURIs: []string{"https://wordpress.example/callback"}, AuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"}}}, IsLeader: func() bool { return *ready }})
	if err != nil {
		t.Fatal(err)
	}
	h.ctx.OAuthProvider = p
	challenge := sha256.Sum256([]byte(strings.Repeat("v", 64)))
	query := url.Values{"response_type": {"code"}, "client_id": {"wordpress"}, "redirect_uri": {"https://wordpress.example/callback"}, "resource": {p.Resource("api")}, "scope": {"sites:read analytics:read"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}, "screen_hint": {"signup"}}
	response := httptest.NewRecorder()
	p.Authorize(response, httptest.NewRequest(http.MethodGet, p.Issuer()+"/oauth/authorize?"+query.Encode(), nil))
	if response.Code != http.StatusSeeOther {
		t.Fatalf("authorize: %d %s", response.Code, response.Body.String())
	}
	return response.Result().Cookies()
}

func TestSocialSignupConfirmationResumesOAuth(t *testing.T) {
	for _, tc := range []struct{ name, plan, path string }{
		{"ordinary", "free", "/dashboard"},
		{"valid", "free", "/oauth/consent"},
		{"forged", "free", "/dashboard"},
		{"paid", "business", "/signup/verified?billing=monthly&plan=business"},
		{"outage", "free", "/oauth/consent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			t.Cleanup(func() { _ = store.Close() })
			h.ctx.Config.CloudHosted = true
			h.ctx.Config.CloudSignupEnabled = true
			h.ctx.Config.SocialSignupEnabled = true
			h.ctx.Config.CloudJurisdiction = "US"
			ready := true
			cookies := socialOAuthCookies(t, h, store, &ready)
			token, err := store.CreatePendingSocialConfirmation(t.Context(), database.PendingSocialConfirmation{Provider: "microsoft", Subject: "11111111-2222-3333-4444-555555555555:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", ObservedEmail: "mutable@example.net", TargetEmail: "signup-oauth@example.test", TeamName: "WordPress", Jurisdiction: "US", Locale: "en", PlanCode: tc.plan, BillingInterval: "monthly", AcceptedTosAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodGet, "/api/auth/social/confirm?token="+url.QueryEscape(token), nil)
			if tc.name != "ordinary" {
				for _, cookie := range cookies {
					if tc.name == "forged" && cookie.Name == "hk_oauth_pending" {
						cookie.Value = "forged"
					}
					r.AddCookie(cookie)
				}
			}
			if tc.name == "outage" {
				ready = false
				w := httptest.NewRecorder()
				h.handleSocialConfirmation()(w, r)
				if w.Code != http.StatusServiceUnavailable {
					t.Fatalf("outage lost confirmation: %d %s", w.Code, w.Header().Get("Location"))
				}
				user, err := store.GetUserByEmail(t.Context(), "signup-oauth@example.test")
				if err != nil || user != nil {
					t.Fatalf("outage consumed confirmation: %v %v", user, err)
				}
				ready = true
			}
			w := httptest.NewRecorder()
			h.handleSocialConfirmation()(w, r)
			if w.Code != http.StatusSeeOther || w.Header().Get("Location") != h.ctx.Config.PublicURL+tc.path {
				t.Fatalf("redirect: %d %q want %q", w.Code, w.Header().Get("Location"), tc.path)
			}
		})
	}
}

func TestSocialSignupCancellationKeepsValidatedOAuthMarker(t *testing.T) {
	for _, mode := range []string{"ordinary", "valid", "forged"} {
		t.Run(mode, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			t.Cleanup(func() { _ = store.Close() })
			ready := true
			cookies := socialOAuthCookies(t, h, store, &ready)
			h.ctx.SocialAuth = socialauth.NewClient(nil)
			state := h.ctx.AuthState.CreateSocialOAuthState(shared.SocialOAuthState{Provider: "github", Flow: "signup", ExpiresAt: time.Now().Add(time.Minute)})
			binding := httptest.NewRecorder()
			h.setSocialStateCookie(binding, state)
			r := httptest.NewRequest(http.MethodGet, "/api/auth/social/github/callback?state="+url.QueryEscape(state)+"&error=access_denied", nil)
			r.SetPathValue("provider", "github")
			for _, cookie := range binding.Result().Cookies() {
				r.AddCookie(cookie)
			}
			if mode != "ordinary" {
				for _, cookie := range cookies {
					if mode == "forged" && cookie.Name == "hk_oauth_pending" {
						cookie.Value = "forged"
					}
					r.AddCookie(cookie)
				}
			}
			w := httptest.NewRecorder()
			h.handleSocialCallback()(w, r)
			want := h.ctx.Config.PublicURL + "/signup?error=social_provider_cancelled"
			if mode == "valid" {
				want += "&oauth=1"
			}
			if w.Code != http.StatusSeeOther || w.Header().Get("Location") != want {
				t.Fatalf("redirect: %d %q want %q", w.Code, w.Header().Get("Location"), want)
			}
		})
	}
}
