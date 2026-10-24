package oauth

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	googleuuid "github.com/google/uuid"
	"hitkeep/server/shared"
)

func TestConsentRequiresAccountSessionNotUserIDAlone(t *testing.T) {
	h := handler{ctx: &shared.Context{}}
	r := httptest.NewRequest(http.MethodGet, "https://hitkeep.example/api/oauth/consent", nil)
	r = r.WithContext(context.WithValue(r.Context(), shared.UserIDKey, googleuuid.New()))
	w := httptest.NewRecorder()
	if _, ok := h.humanUser(w, r); ok || w.Code != 401 {
		t.Fatal("non-session identity reached consent")
	}
}
func TestContinuationWithoutProviderReturnsFalse(t *testing.T) {
	h := handler{ctx: &shared.Context{}}
	w := httptest.NewRecorder()
	h.continuation(w, httptest.NewRequest(http.MethodGet, "https://hitkeep.example/api/oauth/continuation", nil), uuid.NewV7())
	var result struct {
		Pending bool `json:"pending"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Pending {
		t.Fatalf("ordinary account completion broken: %d %s %v", w.Code, w.Body.String(), err)
	}
}

func TestContinuationWithUnavailableProviderReturnsSafeError(t *testing.T) {
	h := handler{ctx: &shared.Context{}}
	r := httptest.NewRequest(http.MethodGet, "https://hitkeep.example/api/oauth/continuation", nil)
	r.AddCookie(&http.Cookie{Name: "hk_oauth_pending", Value: "pending-navigation-state"})
	w := httptest.NewRecorder()
	h.continuation(w, r, uuid.NewV7())
	if w.Code != 503 || w.Body.String() != `{"error":"unavailable"}` {
		t.Fatalf("unsafe outage response: %d %s", w.Code, w.Body.String())
	}
}
