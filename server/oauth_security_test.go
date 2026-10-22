package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/entitlements"
)

func TestServerPrivateDashboardFramingPolicy(t *testing.T) {
	for _, prefix := range []string{"", "/hitkeep"} {
		t.Run(prefix, func(t *testing.T) {
			conf := testServerConfig(t)
			conf.PublicURL = "https://analytics.example" + prefix
			store := testServerStore(t)
			t.Cleanup(func() { _ = store.Close() })
			srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil, testServerLogger())
			t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })
			for _, path := range []string{"/", "/index.html", "/dashboard", "/login", "/reset-password", "/oauth/consent", "/settings/profile"} {
				r := httptest.NewRequest(http.MethodGet, conf.PublicURL+path, nil)
				r.Header.Set("Accept", "text/html")
				w := httptest.NewRecorder()
				srv.httpServer.Handler.ServeHTTP(w, r)
				if w.Code != http.StatusOK || w.Header().Get("Content-Security-Policy") != "frame-ancestors 'none'" || w.Header().Get("X-Frame-Options") != "DENY" {
					t.Errorf("unprotected private shell %s: %d %v", path, w.Code, w.Header())
				}
			}
			for _, path := range []string{"/share/token/dashboard", "/qr-share/token"} {
				r := httptest.NewRequest(http.MethodGet, conf.PublicURL+path, nil)
				r.Header.Set("Accept", "text/html")
				w := httptest.NewRecorder()
				srv.httpServer.Handler.ServeHTTP(w, r)
				if w.Code != http.StatusOK || w.Header().Get("Content-Security-Policy") != "" || w.Header().Get("X-Frame-Options") != "" {
					t.Errorf("public embedding policy changed %s: %d %v", path, w.Code, w.Header())
				}
			}
		})
	}
}

func TestServerMCPRejectsInvalidOriginAtConfiguredMount(t *testing.T) {
	for _, prefix := range []string{"", "/hitkeep"} {
		t.Run(prefix, func(t *testing.T) {
			conf := testServerConfig(t)
			conf.PublicURL = "https://analytics.example" + prefix
			conf.MCPPath = "/assist/tools"
			conf.MCPEnabled = true
			store := testServerStore(t)
			t.Cleanup(func() { _ = store.Close() })
			srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil, testServerLogger())
			t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })
			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions} {
				for _, origin := range []string{"https://evil.example", "null", "", "http://analytics.example", "https://analytics.example.evil", "https://analytics.example/path", "https://analytics.example?", "https://analytics.example#", "https://user@analytics.example", "https://analytics.example:444", "https://analytics.example https://evil.example"} {
					r := httptest.NewRequest(method, conf.PublicURL+conf.MCPPath, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
					r.Header.Set("Origin", origin)
					r.Header.Set("Sec-Fetch-Site", "cross-site")
					w := httptest.NewRecorder()
					srv.httpServer.Handler.ServeHTTP(w, r)
					if w.Code != http.StatusForbidden {
						t.Errorf("%s origin %q: got %d, want 403", method, origin, w.Code)
					}
				}
			}
			for _, origins := range [][]string{nil, {"https://analytics.example"}, {"https://ANALYTICS.example:443"}, {"https://analytics.example", "https://evil.example"}} {
				r := httptest.NewRequest(http.MethodPost, conf.PublicURL+conf.MCPPath, nil)
				for _, origin := range origins {
					r.Header.Add("Origin", origin)
				}
				w := httptest.NewRecorder()
				srv.httpServer.Handler.ServeHTTP(w, r)
				want := http.StatusUnauthorized
				if len(origins) > 1 {
					want = http.StatusForbidden
				}
				if w.Code != want {
					t.Errorf("origins %v: got %d, want %d", origins, w.Code, want)
				}
			}
		})
	}
}

func TestServerMCPPathCannotBypassDashboardOriginProtection(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		conf := testServerConfig(t)
		conf.PublicURL = "https://analytics.example/hitkeep"
		conf.MCPPath = "/api/login"
		conf.MCPEnabled = enabled
		store := testServerStore(t)
		t.Cleanup(func() { _ = store.Close() })
		srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil, testServerLogger())
		t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })
		r := httptest.NewRequest(http.MethodPost, conf.PublicURL+"/api/login", nil)
		r.Header.Set("Origin", "https://evil.example")
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("MCP enabled=%v bypassed dashboard origin policy: %d", enabled, w.Code)
		}
	}
}
