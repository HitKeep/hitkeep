package sites

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"hitkeep/api"
	"hitkeep/server/shared"
)

func TestEcommerceHTTPRejectsMixedCurrencies(t *testing.T) {
	h, store, _ := setupTestEnv(t)
	defer store.Close()
	user, err := store.CreateUser(t.Context(), "mixed-currency@test.dev", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(t.Context(), user, "mixed-currency.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, currency := range []string{"EUR", "USD"} {
		if err := store.CreateEvent(t.Context(), &api.Event{SiteID: site.ID, SessionID: uuid.New(), Timestamp: time.Now().UTC(), Name: "purchase", Properties: map[string]any{"transaction_id": currency, "value": 100, "currency": currency}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, handler := range []http.HandlerFunc{h.handleGetSiteEcommerceSummary(), h.handleGetSiteEcommerceTimeseries(), h.handleGetSiteEcommerceProducts(), h.handleGetSiteEcommerceSources()} {
		r := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/ecommerce", nil)
		r.SetPathValue("id", site.ID.String())
		r = r.WithContext(context.WithValue(r.Context(), shared.UserIDKey, user))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("mixed-currency response=%d %s", w.Code, w.Body.String())
		}
	}
}
