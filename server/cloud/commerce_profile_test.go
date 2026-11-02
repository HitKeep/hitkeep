//go:build billing

package cloud

import (
	"net/http"
	"testing"
)

func TestCommerceProfileRejectsUnusableNegotiation(t *testing.T) {
	for _, raw := range []string{`{}`, `{"ucp":{"version":"2026-01-11"}}`, `{"ucp":{"version":"2026-08-25","capabilities":{"dev.ucp.shopping.checkout":[{"version":"2026-01-11"}]}}}`} {
		if status, err := validateCommerceProfile([]byte(raw)); err == nil || status != http.StatusUnprocessableEntity {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, header := range []string{"", `profile="http://example.com/.well-known/ucp"`, `profile="https://user:pass@example.com/.well-known/ucp"`, `profile="https://example.com:8443/.well-known/ucp"`, `profile="https://example.com/.well-known/ucp?private=1"`, `profile="https://example.com/profile"`, `profile="https://127.0.0.1/.well-known/ucp"`, `profile="https://169.254.169.254/.well-known/ucp"`} {
		if _, err := fetchCommerceProfile(t.Context(), header); err == nil {
			t.Fatalf("accepted unsafe profile %s", header)
		}
	}
}
