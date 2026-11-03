package server

import "net/http"

// The marker can only restrict existing handlers; authentication still belongs to each route.
func (s *Server) agentRequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Header.Values("X-Hitkeep-Agent")) > 0 && !s.agentAvailable() {
			w.Header().Set("Cache-Control", "no-store")
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
