//go:build billing

package cloud

import (
	"hitkeep/config"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudPlansAvailabilityMatchesBuildAndGuard(t *testing.T) {
	if PlansAvailable(nil) || PlansAvailable(&config.Config{}) || !PlansAvailable(&config.Config{CloudHosted: true}) {
		t.Fatal("Cloud plan exposure guard mismatch")
	}
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/.well-known/api-catalog", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatal("Cloud handler still owns shared discovery")
	}
}
