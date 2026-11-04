package shared

import (
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hitkeep/auth"
	"hitkeep/config"
	"hitkeep/database"
)

func TestRequireAuthRejectsCopiedJWTAfterUserDeletion(t *testing.T) {
	store := newSharedTestStore(t)
	userID, err := store.CreateUserWithoutDefaultTenant(t.Context(), "session@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	c := &Context{Store: store, Config: &config.Config{JWTSecret: "live-human-test-secret", PublicURL: "https://example.test"}}
	token, err := auth.GenerateToken(c.Config.JWTSecret, c.Config.PublicURL, userID)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	next := func(w http.ResponseWriter, r *http.Request) {
		called = true
		if GetUserIDFromContext(r) != userID || r.Context().Value(AuthSessionKey) == nil {
			t.Error("missing human session context")
		}
		w.WriteHeader(http.StatusNoContent)
	}
	for _, deleted := range []bool{false, true} {
		if deleted {
			if err := store.DeleteUser(t.Context(), userID); err != nil {
				t.Fatal(err)
			}
			if _, err := auth.ValidateTokenClaims(token, c.Config.JWTSecret, c.Config.PublicURL); err != nil {
				t.Fatalf("copied JWT must still be cryptographically valid: %v", err)
			}
		}
		for _, humanOnly := range []bool{false, true} {
			called = false
			r := httptest.NewRequest(http.MethodGet, "/api/user", nil)
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			r.AddCookie(&http.Cookie{Name: auth.RememberMeCookieName, Value: "old-remember-cookie"})
			w := httptest.NewRecorder()
			c.Handler(HandlerConfig{RequireAuth: true, HumanOnly: humanOnly}, next)(w, r)
			if deleted {
				if w.Code != http.StatusUnauthorized || called {
					t.Fatalf("deleted human reached handler/context: status=%d called=%v", w.Code, called)
				}
				assertClearedHumanCookies(t, w)
			} else if w.Code != http.StatusNoContent || !called {
				t.Fatalf("live human rejected: status=%d called=%v", w.Code, called)
			}
		}
	}
}

func TestRequireAuthRememberRestorationRequiresLiveUser(t *testing.T) {
	for _, state := range []string{"live", "missing", "deleted", "database unavailable"} {
		t.Run(state, func(t *testing.T) {
			store := newSharedTestStore(t)
			userID, err := store.CreateUserWithoutDefaultTenant(t.Context(), "remember@example.test", "hash")
			if err != nil {
				t.Fatal(err)
			}
			token, err := store.CreateRememberMeToken(t.Context(), userID)
			if err != nil {
				t.Fatal(err)
			}
			if state == "missing" {
				// Model a stale restored credential independently of FK-backed lifecycle cleanup.
				if err := store.Exec(t.Context(), "CREATE TABLE stale_remember AS SELECT * FROM remember_me_tokens"); err != nil {
					t.Fatal(err)
				}
				if err := store.Exec(t.Context(), "DROP TABLE remember_me_tokens"); err != nil {
					t.Fatal(err)
				}
				if err := store.Exec(t.Context(), "ALTER TABLE stale_remember RENAME TO remember_me_tokens"); err != nil {
					t.Fatal(err)
				}
				if err := store.Exec(t.Context(), "DELETE FROM users WHERE id = ?", userID); err != nil {
					t.Fatal(err)
				}
			}
			if state == "database unavailable" {
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if state == "deleted" {
				if err := store.DeleteUser(t.Context(), userID); err != nil {
					t.Fatal(err)
				}
			}
			c := &Context{Store: store, Config: &config.Config{JWTSecret: "remember-test-secret", PublicURL: "https://example.test"}}
			r := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
			r.AddCookie(&http.Cookie{Name: auth.RememberMeCookieName, Value: token})
			w := httptest.NewRecorder()
			called := false
			c.RequireAuth(false, func(w http.ResponseWriter, r *http.Request) {
				called = true
				if GetUserIDFromContext(r) != userID || r.Context().Value(AuthSessionKey) == nil {
					t.Error("missing restored human session context")
				}
				w.WriteHeader(http.StatusNoContent)
			})(w, r)
			switch state {
			case "live":
				if w.Code != http.StatusNoContent || !called {
					t.Fatalf("live remember restoration failed: %d", w.Code)
				}
				cookies := w.Result().Cookies()
				if len(cookies) != 1 || cookies[0].Name != auth.CookieName {
					t.Fatalf("expected restored JWT cookie, got %v", cookies)
				}
				claims, err := auth.ValidateTokenClaims(cookies[0].Value, c.Config.JWTSecret, c.Config.PublicURL)
				if err != nil || claims.UserID != userID {
					t.Fatalf("invalid restored JWT: %v", err)
				}
			case "missing", "deleted":
				if w.Code != http.StatusUnauthorized || called {
					t.Fatalf("absent remembered user authenticated: %d", w.Code)
				}
				assertClearedHumanCookies(t, w)
			case "database unavailable":
				if w.Code != http.StatusServiceUnavailable || called || w.Body.String() != "Service not available\n" || len(w.Result().Cookies()) != 0 {
					t.Fatalf("remember database error did not fail closed: %d %q", w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestRequireAuthUserLookupUnavailable(t *testing.T) {
	for _, unavailableStore := range []bool{false, true} {
		t.Run(map[bool]string{false: "database error", true: "no store"}[unavailableStore], func(t *testing.T) {
			store := newSharedTestStore(t)
			userID, err := store.CreateUserWithoutDefaultTenant(t.Context(), "unavailable@example.test", "hash")
			if err != nil {
				t.Fatal(err)
			}
			c := &Context{Store: store, Config: &config.Config{JWTSecret: "unavailable-test-secret", PublicURL: "https://example.test"}}
			token, err := auth.GenerateToken(c.Config.JWTSecret, c.Config.PublicURL, userID)
			if err != nil {
				t.Fatal(err)
			}
			if unavailableStore {
				c.Store = nil
			} else if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			w := httptest.NewRecorder()
			called := false
			c.RequireAuth(false, func(http.ResponseWriter, *http.Request) { called = true })(w, r)
			if w.Code != http.StatusServiceUnavailable || called || w.Body.String() != "Service not available\n" || len(w.Result().Cookies()) != 0 {
				t.Fatalf("lookup failure did not fail closed: status=%d called=%v body=%q", w.Code, called, w.Body.String())
			}
		})
	}
}

func TestRequireAuthPreservesTeamAPIClientAuthentication(t *testing.T) {
	store := newSharedTestStore(t)
	userID, err := store.CreateUser(t.Context(), "api-owner@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.GetActiveTenantID(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	client, token, err := store.CreateTeamAPIClient(t.Context(), teamID, "Team integration", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &Context{Store: store, Config: &config.Config{PublicURL: "https://example.test"}}
	next := func(w http.ResponseWriter, r *http.Request) {
		got, _ := r.Context().Value(APIClientAuthKey).(*database.APIClientAuth)
		if got == nil || got.ClientID != client.ID || got.TenantID != teamID || r.Context().Value(AuthSessionKey) != nil {
			t.Error("team API client lost its credential context")
		}
		w.WriteHeader(http.StatusNoContent)
	}
	for _, handler := range []http.HandlerFunc{c.RequireAuth(true, next), c.RequireIntegrationAuth("sites:read", next)} {
		r := httptest.NewRequest(http.MethodGet, "/api/integrations/sites", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("team API credential rejected: %d %s", w.Code, w.Body.String())
		}
	}
}

func assertClearedHumanCookies(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	cookies := w.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected both human cookies cleared, got %v", cookies)
	}
	for i, name := range []string{auth.CookieName, auth.RememberMeCookieName} {
		cookie := cookies[i]
		if cookie.Name != name || cookie.Value != "" || !cookie.Expires.Before(time.Now()) || !cookie.Secure || !cookie.HttpOnly {
			t.Errorf("cookie %s was not securely cleared: %v", name, cookie)
		}
	}
}

func TestAuthSessionIDIsAdditiveAndRememberRestorationNeverProvesDeletion(t *testing.T) {
	store := newSharedTestStore(t)
	id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "proof-session@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	c := &Context{Store: store, AuthState: NewAuthStateStore(), Config: &config.Config{JWTSecret: "proof-jti-test", PublicURL: "https://example.test"}}
	attempt := c.AuthState.CreateAccountDeletionAttempt(id, c.Config.PublicURL, strings.Repeat("a", 64), "/settings", time.Now().Add(time.Minute))
	token, err := auth.GenerateToken(c.Config.JWTSecret, c.Config.PublicURL, id)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := auth.ValidateTokenClaims(token, c.Config.JWTSecret, c.Config.PublicURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		if legacy {
			claims.ID = ""
			token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(c.Config.JWTSecret))
			if err != nil {
				t.Fatal(err)
			}
		}
		request := httptest.NewRequest("GET", "/api/auth/session", nil)
		request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		out := httptest.NewRecorder()
		c.RequireHumanAuth(func(w http.ResponseWriter, r *http.Request) {
			session := r.Context().Value(AuthSessionKey).(AuthSessionContext)
			if (session.ID == "") != legacy {
				t.Error("legacy session ID compatibility")
			}
			if c.AuthState.AccountDeletionProofVerified(attempt, id, session.ID, strings.Repeat("a", 64)) {
				t.Error("cookie authentication created proof")
			}
			w.WriteHeader(204)
		})(out, request)
		if out.Code != 204 {
			t.Fatalf("ordinary legacy=%v session rejected %d", legacy, out.Code)
		}
	}
	remembered, err := store.CreateRememberMeToken(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/auth/session", nil)
	request.AddCookie(&http.Cookie{Name: auth.RememberMeCookieName, Value: remembered})
	out := httptest.NewRecorder()
	c.RequireHumanAuth(func(w http.ResponseWriter, r *http.Request) {
		session := r.Context().Value(AuthSessionKey).(AuthSessionContext)
		if session.ID == "" {
			t.Error("restored session missing internal ID")
		}
		if c.AuthState.AccountDeletionProofVerified(attempt, id, session.ID, strings.Repeat("a", 64)) {
			t.Error("remember restoration created proof")
		}
		w.WriteHeader(204)
	})(out, request)
	if out.Code != 204 {
		t.Fatalf("remember restoration %d", out.Code)
	}
}
