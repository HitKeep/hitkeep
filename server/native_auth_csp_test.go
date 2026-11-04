package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/server/shared"
)

func TestNativeConsentCSPAllowsOnlyRegisteredCallbacks(t *testing.T) {
	server := &Server{indexHTML: []byte("<html>native consent fixture</html>")}
	for _, path := range []string{"/auth/native/consent", "/login", "/signup", "/reset-password", "/settings"} {
		response := httptest.NewRecorder()
		server.serveIndex(response, httptest.NewRequest("GET", path, nil))
		policy := response.Header().Get("Content-Security-Policy")
		if path == "/auth/native/consent" {
			expected := "form-action 'self' " + shared.NativeIOSCallback + " " + shared.NativeAndroidCallback + ";"
			if !strings.Contains(policy, expected) {
				t.Errorf("native consent cannot complete exact HTTPS callbacks: %s", policy)
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Error("native consent cache/referrer policy weakened")
			}
		} else if strings.Contains(policy, "hitkeep.com") {
			t.Errorf("native callback CSP escaped onto %s", path)
		}
		if strings.Contains(policy, "form-action *") || strings.Contains(policy, "form-action https:") {
			t.Error("broad form-action")
		}
	}
}
