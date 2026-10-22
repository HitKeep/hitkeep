//go:build billing

package cloud

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"hitkeep/api"
	"hitkeep/config"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

func TestCloudPlanCatalogHumanApprovedReviewURLs(t *testing.T) {
	for _, region := range []struct{ jurisdiction, currency, publicURL, scheme, host string }{
		{"EU", "EUR", "https://analytics.example/prefix/", "https", "analytics.example"},
		{"US", "USD", "https://analytics.example:443/prefix/", "https", "analytics.example:443"},
		{"EU", "EUR", "http://analytics.example:8080/prefix/", "http", "analytics.example:8080"},
	} {
		t.Run(region.publicURL, func(t *testing.T) {
			h, provider := planCatalogTestHandler()
			h.ctx.Config.CloudJurisdiction = region.jurisdiction
			h.ctx.Config.PublicURL = region.publicURL
			h.ctx.Config.StripePriceProAnnual = "pro_year"
			h.ctx.Config.StripePriceBusinessMonthly = "business_month"
			h.ctx.Config.StripePriceBusinessAnnual = "business_year"
			for _, offer := range []struct{ id, interval string }{{"pro_month", "month"}, {"pro_year", "year"}, {"business_month", "month"}, {"business_year", "year"}} {
				provider.prices[offer.id] = &stripePriceOutput{AmountMinor: 17500, Currency: region.currency, Interval: offer.interval, Active: true}
			}
			plans, body := readPlanCatalog(t, h, provider)
			if len(plans) != 3 {
				t.Fatalf("plan count = %d, want 3", len(plans))
			}
			for _, plan := range plans {
				if plan.Code == "free" {
					if len(plan.Prices) != 0 {
						t.Fatal("Free plan must not advertise a paid offer")
					}
					continue
				}
				if plan.Code != "pro" && plan.Code != "business" {
					t.Fatalf("unexpected plan %q", plan.Code)
				}
				if len(plan.Prices) != 2 {
					t.Fatalf("%s prices = %v", plan.Code, plan.Prices)
				}
				for _, interval := range []string{"monthly", "annual"} {
					price := plan.Prices[interval]
					if price.AmountMinor != 17500 || price.Currency != region.currency {
						t.Fatalf("unexpected price: %+v", price)
					}
					review, err := url.Parse(price.PurchaseReviewURL)
					if err != nil {
						t.Fatal(err)
					}
					if review.Scheme != region.scheme || review.Host != region.host || review.Path != "/prefix/admin/team" || review.User != nil || review.Fragment != "" {
						t.Fatalf("noncanonical review URL %q", price.PurchaseReviewURL)
					}
					query := review.Query()
					if len(query) != 3 || query.Get("purchase") != "review" || query.Get("plan") != plan.Code || query.Get("billing") != interval {
						t.Fatalf("unexpected review query %q", review.RawQuery)
					}
				}
			}
			for _, private := range []string{"attacker.example", "tenant_id", "customer_id", "checkout_session", "fixture-secret"} {
				if strings.Contains(body, private) {
					t.Fatalf("catalog leaked %q", private)
				}
			}
		})
	}
}

func TestCloudPlanCatalogOmitsUnavailableOffers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*handler, *fakeStripeClient)
	}{
		{"unconfigured", func(h *handler, _ *fakeStripeClient) { h.ctx.Config.StripePriceProMonthly = "" }},
		{"missing key", func(h *handler, _ *fakeStripeClient) { h.ctx.Config.StripeSecretKey = "" }},
		{"missing price", func(_ *handler, p *fakeStripeClient) { p.prices = nil }},
		{"inactive", func(_ *handler, p *fakeStripeClient) { p.prices["pro_month"].Active = false }},
		{"zero amount", func(_ *handler, p *fakeStripeClient) { p.prices["pro_month"].AmountMinor = 0 }},
		{"wrong currency", func(_ *handler, p *fakeStripeClient) { p.prices["pro_month"].Currency = "USD" }},
		{"wrong interval", func(_ *handler, p *fakeStripeClient) { p.prices["pro_month"].Interval = "year" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, provider := planCatalogTestHandler()
			tc.change(h, provider)
			plans, body := readPlanCatalog(t, h, provider)
			for _, plan := range plans {
				if len(plan.Prices) != 0 {
					t.Fatalf("unavailable offer advertised: %+v", plan)
				}
			}
			if strings.Contains(body, "purchase_review_url") {
				t.Fatal("unavailable offers must omit review URLs")
			}
		})
	}
	h, _ := planCatalogTestHandler()
	h.ctx.Config.CloudHosted = false
	response := httptest.NewRecorder()
	h.handleListCloudPlans().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/cloud/plans", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("non-cloud status = %d, want 404", response.Code)
	}
}

func TestCloudPlanCatalogOmitsNoncanonicalReviewURL(t *testing.T) {
	for _, base := range []string{
		"", "/prefix", "javascript:alert(1)", "https://operator:secret@analytics.example", "https://[bad",
		"https://analytics.example/prefix?region=EU", "https://analytics.example/prefix?",
		"https://analytics.example/prefix#section", "https://analytics.example/prefix#",
		"https://:443/prefix", "https://analytics.example:0/prefix", "https://analytics.example:65536/prefix",
		"https://analytics.example:abc/prefix", "https://analytics.example:-1/prefix", "https://analytics.example:/prefix",
		"https://analytics.example:80:443/prefix",
	} {
		t.Run(base, func(t *testing.T) {
			h, provider := planCatalogTestHandler()
			h.ctx.Config.PublicURL = base
			plans, body := readPlanCatalog(t, h, provider)
			if len(plans) != 3 || len(plans[1].Prices) != 1 || plans[1].Prices["monthly"].AmountMinor != 17500 {
				t.Fatalf("existing price contract changed: %+v", plans)
			}
			if strings.Contains(body, "purchase_review_url") {
				t.Fatal("missing or invalid canonical base must omit review URL")
			}
		})
	}
}

func planCatalogTestHandler() (*handler, *fakeStripeClient) {
	provider := &fakeStripeClient{prices: map[string]*stripePriceOutput{"pro_month": {AmountMinor: 17500, Currency: "EUR", Interval: "month", Active: true}}}
	return &handler{ctx: &shared.Context{Config: &config.Config{CloudHosted: true, PublicURL: "https://analytics.example/prefix/", CloudJurisdiction: "EU", StripeSecretKey: "fixture-secret", StripePriceProMonthly: "pro_month"}}, stripe: provider}, provider
}

func readPlanCatalog(t *testing.T, h *handler, provider *fakeStripeClient) ([]api.CloudPlanTier, string) {
	t.Helper()
	response := httptest.NewRecorder()
	h.handleListCloudPlans().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://attacker.example/api/cloud/plans", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("catalog status = %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	var plans []api.CloudPlanTier
	if err := json.UnmarshalRead(response.Body, &plans); err != nil {
		t.Fatal(err)
	}
	if provider.lastCustomerInput != nil || provider.lastCheckoutInput != nil || provider.lastPortalInput != nil || provider.lastChargeID != "" {
		t.Fatal("public catalog invoked a provider mutation or customer lookup")
	}
	return plans, body
}
