package server

import (
	"hitkeep/server/shared"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentRequestMarkerOnlyRestricts(t *testing.T) {
	s := newDiscoveryServer()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, enabled := range []bool{false, true} {
		s.conf.AIEnabled = enabled
		for _, marked := range []bool{false, true} {
			r := httptest.NewRequest("POST", "/api/sites", nil)
			if marked {
				r.Header.Set("X-HitKeep-Agent", "webmcp")
			}
			w := httptest.NewRecorder()
			s.agentRequestMiddleware(next).ServeHTTP(w, r)
			want := 204
			if marked && !enabled {
				want = 404
			}
			if w.Code != want {
				t.Fatalf("AI=%v marker=%v got %d", enabled, marked, w.Code)
			}
		}
	}
	r := httptest.NewRequest("POST", "/api/sites", nil)
	r.Header.Set("X-HitKeep-Agent", "webmcp")
	protected := s.ctx.Handler(shared.HandlerConfig{RequireAuth: true}, next.ServeHTTP)
	w := httptest.NewRecorder()
	s.agentRequestMiddleware(protected).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("marker granted authority: %d", w.Code)
	}
}
