package server

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"hitkeep/server/shared"
)

func TestNativeConsentCSPAllowsOnlyRegisteredCallbacks(t *testing.T) {
	server := &Server{indexHTML: []byte("<html>native consent fixture</html>")}
	for _, path := range []string{"/auth/native/manage", "/auth/native/consent", "/login", "/signup", "/reset-password", "/auth/native/manage/extra", "/settings"} {
		response := httptest.NewRecorder()
		server.serveIndex(response, httptest.NewRequest("GET", path, nil))
		policy := response.Header().Get("Content-Security-Policy")
		if path == "/auth/native/consent" || path == "/auth/native/manage" {
			expected := "form-action 'self' " + shared.NativeIOSCallback + " " + shared.NativeAndroidCallback + ";"
			if !strings.Contains(policy, expected) {
				t.Errorf("native consent cannot complete exact HTTPS callbacks: %s", policy)
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "same-origin" {
				t.Error("native consent cache/referrer policy weakened")
			}
		} else {
			if strings.Contains(policy, "hitkeep.com") {
				t.Errorf("native callback CSP escaped onto %s", path)
			}
			if path != "/settings" && response.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Errorf("same-origin referrer policy escaped onto %s", path)
			}
		}
		if strings.Contains(policy, "form-action *") || strings.Contains(policy, "form-action https:") {
			t.Error("broad form-action")
		}
	}
}

func TestAuthCSPAllowsBundledAngularBootstrapHashes(t *testing.T) {
	const hash = "'sha256-9csbfYiTiLlYfccT+QDMUV43Kb7i94TLRQWwclf07o8='"
	server := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), publicBasePath: "/prefix/"}
	server.loadStaticAssets(fstest.MapFS{"index.html": {Data: []byte(`<meta http-equiv="Content-Security-Policy" content="script-src 'strict-dynamic' ` + hash + ` https: 'unsafe-inline';"><base href="/">`)}, "scalar/index.html": {Data: []byte("docs")}})
	for _, path := range []string{"/login", "/signup", "/auth/native/consent", "/auth/native/manage"} {
		rr := httptest.NewRecorder()
		server.serveIndex(rr, httptest.NewRequest("GET", path, nil))
		policy := rr.Header().Get("Content-Security-Policy")
		if !strings.Contains(policy, "script-src 'self' "+hash+";") {
			t.Errorf("bundled bootstrap blocked on %s: %s", path, policy)
		}
		if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") || strings.Contains(policy, "script-src 'self' https:") {
			t.Error("script trust broadened beyond bundled hashes")
		}
	}
}
