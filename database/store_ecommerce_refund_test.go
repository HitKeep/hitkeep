package database

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

func TestRefundItemsDoNotInventRefundedUnits(t *testing.T) {
	items := normalizeEcommerceRefundItems(map[string]any{"items": []any{
		map[string]any{"item_id": "amount-only", "quantity": float64(0), "price": float64(5)},
		map[string]any{"item_id": "missing-quantity", "price": float64(5)},
		map[string]any{"item_id": "returned-unit", "quantity": float64(1), "price": float64(10)},
	}})
	if len(items) != 1 || items[0].itemID != "returned-unit" || items[0].quantity != 1 {
		t.Fatalf("invented refunded units: %+v", items)
	}
}

func TestEcommerceRefundsAdjustRevenueWithoutCreatingOrders(t *testing.T) {
	store := newSharedTestFixtureStore(t)
	ctx := t.Context()
	user, err := store.CreateUser(ctx, "refunds@test.dev", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, user, "refunds.test")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	session := uuid.New()
	if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: session, PageID: uuid.New(), Path: "/product", Timestamp: start, UTMSource: new("newsletter")}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name     string
		value    float64
		offset   time.Duration
		refund   string
		quantity int
	}{
		{"purchase", 100, 0, "", 2},
		{"refund", 25, 24 * time.Hour, "r1", 1},
		{"refund", 75, 48 * time.Hour, "r2", 1},
	} {
		props := map[string]any{"transaction_id": "order1", "value": row.value, "currency": "EUR", "refund_id": row.refund, "items": []map[string]any{{"item_id": "sku", "quantity": row.quantity, "price": row.value / float64(row.quantity)}}}
		if err := store.CreateEvent(ctx, &api.Event{SiteID: site.ID, SessionID: session, Name: row.name, Timestamp: start.Add(row.offset), Properties: props}); err != nil {
			t.Fatal(err)
		}
	}
	params := api.EcommerceParams{SiteID: site.ID, Start: start.Add(-time.Hour), End: start.Add(72 * time.Hour)}
	summary, err := store.GetEcommerceSummary(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Orders != 1 || summary.Revenue != 0 || summary.Currency != "EUR" {
		t.Fatalf("refunded order: %+v", summary)
	}
	series, err := store.GetEcommerceTimeSeries(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	var revenue float64
	var orders int
	for _, point := range series {
		revenue += point.Revenue
		orders += point.Orders
	}
	if revenue != 0 || orders != 1 {
		t.Fatalf("series totals: revenue=%v orders=%d", revenue, orders)
	}
	products, err := store.GetEcommerceTopProducts(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].Revenue != 0 || products[0].Quantity != 0 || products[0].Orders != 1 {
		t.Fatalf("products: %+v", products)
	}
	sources, err := store.GetEcommerceSources(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Revenue != 0 || sources[0].Orders != 1 {
		t.Fatalf("sources: %+v", sources)
	}
	params.Start = start.Add(12 * time.Hour)
	summary, err = store.GetEcommerceSummary(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Orders != 0 || summary.Revenue != -100 {
		t.Fatalf("refund-only window: %+v", summary)
	}
	params.Filters = []api.Filter{{Type: "utm_source", Value: "newsletter"}}
	summary, err = store.GetEcommerceSummary(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Orders != 0 || summary.Revenue != -100 {
		t.Fatalf("delayed refund lost original source: %+v", summary)
	}
	params.Start = start.Add(-time.Hour)
	params.Filters = nil
	if err := store.CreateEvent(ctx, &api.Event{SiteID: site.ID, SessionID: session, Name: "purchase", Timestamp: start, Properties: map[string]any{"transaction_id": "usd-order", "value": 100, "currency": "USD"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetEcommerceSummary(ctx, params); !errors.Is(err, ErrMixedEcommerceCurrencies) {
		t.Fatalf("combined incompatible currencies: %v", err)
	}
}
