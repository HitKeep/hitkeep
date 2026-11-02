//go:build billing

package cloud

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v87"
)

func TestReuseCheckoutUsesCurrentSubscriptionState(t *testing.T) {
	for _, status := range []string{"canceled", "incomplete_expired", "active", "past_due", "unpaid", "incomplete", ""} {
		t.Run(status, func(t *testing.T) {
			h, f := planCatalogTestHandler()
			teamID := uuid.New()
			f.sessions = []*checkoutSessionOutput{
				{ID: "cs_newer_history", TenantID: teamID.String(), Status: "complete", SubscriptionID: "sub_newer", SubscriptionStatus: status},
				{ID: "cs_older_history", TenantID: teamID.String(), Status: "complete", SubscriptionID: "sub_older", SubscriptionStatus: status},
			}
			input := createCheckoutSessionInput{CustomerID: "cus_test", TenantID: teamID, PlanCode: "pro", BillingInterval: "monthly", PriceID: "pro_month"}
			first, err := h.reuseOrCreateCheckout(t.Context(), input)
			if status != "canceled" && status != "incomplete_expired" {
				if !errors.Is(err, errCheckoutConflict) || f.checkoutCalls != 0 {
					t.Fatalf("live/unknown history created checkout: %v calls=%d", err, f.checkoutCalls)
				}
				return
			}
			if err != nil || f.checkoutCalls != 1 {
				t.Fatalf("terminal history prevented resubscribe: %v calls=%d", err, f.checkoutCalls)
			}
			again, err := h.reuseOrCreateCheckout(t.Context(), input)
			if err != nil || again.ID != first.ID || f.checkoutCalls != 1 {
				t.Fatalf("pending resubscribe retry=%v calls=%d", err, f.checkoutCalls)
			}
		})
	}
}

func TestReuseCheckoutChecksAllCustomerSessions(t *testing.T) {
	h, f := planCatalogTestHandler()
	teamID := uuid.New()
	f.sessions = []*checkoutSessionOutput{
		{ID: "cs_pending", TenantID: teamID.String(), Status: "open", PlanCode: "pro", BillingInterval: "monthly", PriceID: "pro_month"},
		{ID: "cs_unreconciled", TenantID: teamID.String(), Status: "complete", SubscriptionID: "sub_active", SubscriptionStatus: "active"},
	}
	input := createCheckoutSessionInput{CustomerID: "cus_test", TenantID: teamID, PlanCode: "pro", BillingInterval: "monthly", PriceID: "pro_month"}
	if _, err := h.reuseOrCreateCheckout(t.Context(), input); !errors.Is(err, errCheckoutConflict) {
		t.Fatalf("pending session hid active subscription: %v", err)
	}
	f.sessions[1] = &checkoutSessionOutput{ID: "cs_competing", TenantID: teamID.String(), Status: "open", PlanCode: "business", BillingInterval: "monthly", PriceID: "business_month"}
	if _, err := h.reuseOrCreateCheckout(t.Context(), input); !errors.Is(err, errCheckoutConflict) {
		t.Fatalf("pending session hid competing checkout: %v", err)
	}
}

func TestStripeCheckoutSessionWireContract(t *testing.T) {
	teamID := uuid.New()
	calls := 0
	fixture := fmt.Sprintf(`{"id":"cs_wire","object":"checkout.session","url":"https://checkout.stripe.test/wire","mode":"subscription","status":"open","payment_status":"unpaid","customer":"cus_wire","currency":"eur","amount_subtotal":1200,"amount_total":1200,"metadata":{"tenant_id":%q,"plan_code":"pro","billing_interval":"monthly","price_id":"price_wire"}}`, teamID.String())
	historical := strings.Replace(fixture, `"status":"open"`, `"status":"complete","subscription":{"id":"sub_old","object":"subscription","status":"canceled"}`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Stripe-Version"); got != stripeAPIVersion {
			t.Errorf("Stripe-Version=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/checkout/sessions":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Header.Get("Idempotency-Key") != "wire-key" || r.Form.Get("mode") != "subscription" || r.Form.Get("line_items[0][price]") != "price_wire" || r.Form.Get("line_items[0][quantity]") != "1" || r.Form.Get("metadata[tenant_id]") != teamID.String() {
				t.Errorf("unsafe create request: %v", r.Form)
			}
			_, _ = w.Write([]byte(fixture))
		case "GET /v1/checkout/sessions":
			if r.URL.Query().Get("customer") != "cus_wire" {
				t.Error("unscoped customer list")
			}
			if r.URL.Query().Get("expand[0]") != "data.subscription" {
				t.Errorf("subscription status not expanded: %s", r.URL.RawQuery)
			}
			_, _ = fmt.Fprintf(w, `{"object":"list","data":[%s],"has_more":false,"url":"/v1/checkout/sessions"}`, historical)
		case "GET /v1/checkout/sessions/cs_wire":
			if r.URL.Query().Get("expand[0]") != "subscription" {
				t.Errorf("subscription status not expanded: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(historical))
		case "POST /v1/checkout/sessions/cs_wire/expire":
			if r.Header.Get("Idempotency-Key") != "cancel-key" {
				t.Error("cancel idempotency missing")
			}
			_, _ = w.Write([]byte(`{"id":"cs_wire","object":"checkout.session","status":"expired","customer":"cus_wire"}`))
		default:
			t.Errorf("unexpected Stripe operation %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: new(server.URL), HTTPClient: server.Client(), MaxNetworkRetries: new(int64(0))})
	client := &stripeSDKClient{client: stripe.NewClient("sk_fixture", stripe.WithBackends(&stripe.Backends{API: backend}))}
	session, err := client.CreateCheckoutSession(t.Context(), createCheckoutSessionInput{IdempotencyKey: "wire-key", CustomerID: "cus_wire", PriceID: "price_wire", TenantID: teamID, PlanCode: "pro", BillingInterval: "monthly"})
	if err != nil {
		t.Fatal(err)
	}
	if session.CustomerID != "cus_wire" || session.TenantID != teamID.String() || session.Status != "open" || session.PriceID != "price_wire" || session.AmountTotal != 1200 {
		t.Fatalf("bad session projection: %+v", session)
	}
	sessions, err := client.ListCheckoutSessions(t.Context(), "cus_wire")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list=%v err=%v", sessions, err)
	}
	if sessions[0].SubscriptionID != "sub_old" || sessions[0].SubscriptionStatus != "canceled" {
		t.Fatalf("missing current subscription state: %+v", sessions[0])
	}
	loaded, err := client.GetCheckoutSession(t.Context(), "cs_wire")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SubscriptionStatus != "canceled" {
		t.Fatalf("retrieve lost subscription state: %+v", loaded)
	}
	expired, err := client.ExpireCheckoutSession(t.Context(), "cs_wire", "cancel-key")
	if err != nil || expired.Status != "expired" {
		t.Fatalf("expire=%v err=%v", expired, err)
	}
	if calls != 4 {
		t.Fatalf("Stripe requests=%d", calls)
	}
}
