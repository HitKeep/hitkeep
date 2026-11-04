package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"uuid"

	"hitkeep/auth"
	"hitkeep/config"
	"hitkeep/server/shared"
)

func TestEmbedFramingIsLimitedToSignedLaunch(t *testing.T) {
	site := uuid.NewV7()
	cfg := &config.Config{PublicURL: "https://hitkeep.example", JWTSecret: "embed-test-secret"}
	srv := &Server{ctx: &shared.Context{Config: cfg}, indexHTML: []byte("<html>dashboard</html>")}
	launch, err := auth.GenerateEmbedLaunch(cfg.JWTSecret, cfg.PublicURL, site, "https://wordpress.example", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		status     int
		parent     string
	}{
		{"ordinary dashboard", "/dashboard", 200, ""},
		{"login", "/login", 200, ""},
		{"missing launch", "/embed/" + site.String(), 401, ""},
		{"forged launch", "/embed/" + site.String() + "?launch=forged", 401, ""},
		{"wrong site", "/embed/" + uuid.NewV7().String() + "?launch=" + url.QueryEscape(launch), 401, ""},
		{"signed launch", "/embed/" + site.String() + "?launch=" + url.QueryEscape(launch), 200, "https://wordpress.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			srv.serveIndex(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.parent != "" {
				if w.Header().Get("Content-Security-Policy") != "frame-ancestors "+tc.parent || w.Header().Get("X-Frame-Options") != "" || w.Header().Get("Referrer-Policy") != "no-referrer" {
					t.Fatalf("embed framing: %v", w.Header())
				}
			} else if tc.status == 200 {
				if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || w.Header().Get("X-Frame-Options") != "DENY" {
					t.Fatalf("ordinary framing relaxed: %v", w.Header())
				}
			}
		})
	}
}
