package shared

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hitkeep/auth"
	"hitkeep/config"
)

func TestRememberRestoreCanonicalHTTPSCookie(t *testing.T) {
	store := newSharedTestStore(t)
	defer store.Close()
	userID, err := store.CreateUser(t.Context(), "remember-secure@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	remember, err := store.CreateRememberMeToken(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	c := &Context{Store: store, Config: &config.Config{PublicURL: "HTTPS://EXAMPLE.test:443/prefix", JWTSecret: "test-secret"}}
	req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
	req.AddCookie(&http.Cookie{Name: auth.RememberMeCookieName, Value: remember})
	rr := httptest.NewRecorder()
	c.RequireAuth(false, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("restore status %d", rr.Code)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != auth.CookieName || !cookies[0].Secure {
		t.Fatalf("insecure restored cookies: %v", cookies)
	}
	claims, err := auth.ValidateTokenClaims(cookies[0].Value, c.Config.JWTSecret, c.Config.PublicURL)
	if err != nil || claims.Issuer != c.Config.PublicURL {
		t.Fatalf("raw issuer changed: %v, %v", claims, err)
	}
}
