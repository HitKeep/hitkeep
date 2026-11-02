//go:build billing

package cloud

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	stripe "github.com/stripe/stripe-go/v87"

	"hitkeep/auth"
	"hitkeep/database"
	"hitkeep/server/shared"
)

func TestCommerceGatesBeforeProvider(t *testing.T) {
	for _, gate := range []string{"ai", "cloud", "store", "provider"} {
		t.Run(gate, func(t *testing.T) {
			h, store := setupCloudTestHandler(t)
			defer store.Close()
			h.ctx.Config.AIEnabled = true
			switch gate {
			case "ai":
				h.ctx.Config.AIEnabled = false
			case "cloud":
				h.ctx.Config.CloudHosted = false
			case "store":
				h.ctx.Store = nil
			case "provider":
				h.ctx.Config.StripeSecretKey = ""
			}
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"line_items":[{"id":"pro:monthly"}],"currency":"eur","capabilities":{}}`))
			w := httptest.NewRecorder()
			h.handleCommerceSession("acp", "create").ServeHTTP(w, req)
			if w.Code < 400 {
				t.Fatalf("gate %s returned %d", gate, w.Code)
			}
			if f := h.stripe.(*fakeStripeClient); f.checkoutCalls != 0 || f.lastCustomerInput != nil {
				t.Fatal("provider mutated")
			}
		})
	}
}

func TestCommerceStateRequiresProviderPayment(t *testing.T) {
	for _, tc := range []struct{ status, payment, want string }{{"open", "unpaid", "requires_escalation"}, {"complete", "unpaid", "complete_in_progress"}, {"complete", "paid", "completed"}, {"expired", "unpaid", "canceled"}} {
		t.Run(tc.status+tc.payment, func(t *testing.T) {
			s := &checkoutSessionOutput{ID: "cs_test", URL: "https://checkout.stripe.test/session", Status: tc.status, PaymentStatus: tc.payment}
			if got := commerceSessionStatus(s); got != tc.want {
				t.Fatalf("status=%s want %s", got, tc.want)
			}
		})
	}
}

func TestBillingCheckoutReusesPendingSession(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	buyer, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{Email: "reuse@example.com", HashedPassword: "hashed", TeamName: "Reuse"})
	if err != nil {
		t.Fatal(err)
	}
	f := h.stripe.(*fakeStripeClient)
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"plan_code":"pro","billing":"monthly"}`))
		req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, buyer.UserID))
		w := httptest.NewRecorder()
		h.handleCreateBillingCheckoutSession().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("checkout=%d %s", w.Code, w.Body.String())
		}
	}
	if f.checkoutCalls != 1 {
		t.Fatalf("created %d sessions", f.checkoutCalls)
	}
}

func TestBillingCheckoutResubscriptionReusesPendingAfterCanceledHistory(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	buyer, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{Email: "resubscribe@example.test", HashedPassword: "hash", TeamName: "Resubscribe"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertCloudBillingAccount(t.Context(), database.CloudBillingAccount{TenantID: buyer.TenantID, PlanCode: database.CloudPlanFree, PlanName: "Free", SubscriptionStatus: database.CloudSubscriptionStatusCanceled, StripeCustomerID: "cus_test", StripeSubscriptionID: "sub_newer"}); err != nil {
		t.Fatal(err)
	}
	f := h.stripe.(*fakeStripeClient)
	f.sessions = []*checkoutSessionOutput{
		{ID: "cs_newer_history", TenantID: buyer.TenantID.String(), Status: "complete", SubscriptionID: "sub_newer", SubscriptionStatus: "canceled"},
		{ID: "cs_older_history", TenantID: buyer.TenantID.String(), Status: "complete", SubscriptionID: "sub_older", SubscriptionStatus: "canceled"},
	}
	for range 2 {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"plan_code":"pro","billing":"monthly"}`))
		req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, buyer.UserID))
		w := httptest.NewRecorder()
		h.handleCreateBillingCheckoutSession().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("resubscribe=%d %s", w.Code, w.Body.String())
		}
	}
	if f.checkoutCalls != 1 {
		t.Fatalf("resubscribe created %d sessions", f.checkoutCalls)
	}
}

func (f *fakeStripeClient) ListCheckoutSessions(_ context.Context, _ string) ([]*checkoutSessionOutput, error) {
	if f.afterCheckoutList != nil {
		f.afterCheckoutList()
	}
	return f.sessions, nil
}

func TestCheckoutRetryPreservesConcurrentSubscriptionWebhook(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	t.Cleanup(func() { _ = store.Close() })
	buyer, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{Email: "checkout-webhook-race@example.test", HashedPassword: "hash", TeamName: "Checkout race"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertCloudBillingAccount(t.Context(), database.CloudBillingAccount{TenantID: buyer.TenantID, PlanCode: database.CloudPlanFree, PlanName: "Free", SubscriptionStatus: database.CloudSubscriptionStatusPendingCheckout, StripeCustomerID: "cus_test"}); err != nil {
		t.Fatal(err)
	}
	f := h.stripe.(*fakeStripeClient)
	f.sessions = []*checkoutSessionOutput{{ID: "cs_issued", URL: "https://checkout.stripe.test/issued", TenantID: buyer.TenantID.String(), Status: "open", PlanCode: "pro", BillingInterval: "monthly"}}
	event := stripe.Event{ID: "evt_checkout_race", Type: "customer.subscription.updated", Data: &stripe.EventData{Raw: []byte(`{"id":"sub_paid","metadata":{"tenant_id":"` + buyer.TenantID.String() + `","plan_code":"pro","plan_name":"Pro"},"customer":{"id":"cus_test"},"status":"active","items":{"data":[{"price":{"id":"price_pro"}}]}}`)}}
	f.afterCheckoutList = func() {
		if err := h.handleStripeEvent(t.Context(), event); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"plan_code":"pro","billing":"monthly"}`))
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, buyer.UserID))
	w := httptest.NewRecorder()
	h.handleCreateBillingCheckoutSession().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("checkout retry: %d %s", w.Code, w.Body.String())
	}
	account, err := store.GetCloudBillingAccount(t.Context(), buyer.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if account.PlanCode != database.CloudPlanPro || account.SubscriptionStatus != database.CloudSubscriptionStatusActive || account.StripeSubscriptionID != "sub_paid" {
		t.Fatalf("checkout retry overwrote paid subscription: %+v", account)
	}
	if ent := h.ctx.Limits().TeamEntitlements(t.Context(), buyer.TenantID); ent.MaxRetentionDays != 365 {
		t.Fatalf("paid entitlement lost: %+v", ent)
	}
}
func (f *fakeStripeClient) GetCheckoutSession(_ context.Context, id string) (*checkoutSessionOutput, error) {
	for _, s := range f.sessions {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, errCheckoutConflict
}
func (f *fakeStripeClient) ExpireCheckoutSession(ctx context.Context, id, _ string) (*checkoutSessionOutput, error) {
	s, err := f.GetCheckoutSession(ctx, id)
	if err == nil {
		s.Status = "expired"
	}
	return s, err
}

func TestCommerceSessionLifecycleAndAuthorization(t *testing.T) {
	for _, protocol := range []string{"acp", "ucp"} {
		t.Run(protocol, func(t *testing.T) {
			h, store := setupCloudTestHandler(t)
			defer store.Close()
			h.ctx.Config.AIEnabled = true
			h.profileCheck = func(context.Context, string) (int, error) {
				return validateCommerceProfile([]byte(`{"ucp":{"version":"2026-08-25","capabilities":{"dev.ucp.shopping.checkout":[{"version":"2026-08-25"}]}}}`))
			}
			buyer, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{Email: protocol + "@example.test", HashedPassword: "hashed", TeamName: "Checkout"})
			if err != nil {
				t.Fatal(err)
			}
			fake := h.stripe.(*fakeStripeClient)
			fake.prices = map[string]*stripePriceOutput{"price_pro": {AmountMinor: 1200, Currency: "EUR", Interval: "month", Active: true}}
			token, err := auth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, buyer.UserID)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			h.registerCommerce(mux)
			base := "/api/cloud/commerce/" + protocol + "/checkout_sessions"
			body := `{"line_items":[{"id":"pro:monthly"}],"currency":"eur","capabilities":{}}`
			if protocol == "ucp" {
				base = "/api/cloud/commerce/ucp/checkout-sessions"
				body = `{"ucp":{"version":"2026-08-25"},"line_items":[{"item":{"id":"pro:monthly"},"quantity":1}]}`
			}
			call := func(method, path, key, payload string, cookie bool, bearer string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, strings.NewReader(payload))
				req.Header.Set("API-Version", acpVersion)
				req.Header.Set("Idempotency-Key", key)
				req.Header.Set("Request-Id", "request-test")
				req.Header.Set("UCP-Agent", `profile="https://agent.example/.well-known/ucp"`)
				if cookie {
					req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
				}
				if bearer != "" {
					req.Header.Set("Authorization", "Bearer "+bearer)
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, req)
				return w
			}
			for _, credential := range []struct {
				cookie bool
				bearer string
			}{{false, ""}, {false, "invalid"}, {true, "invalid"}} {
				if w := call("POST", base, "denied", body, credential.cookie, credential.bearer); w.Code != 401 {
					t.Fatalf("auth=%d %s", w.Code, w.Body.String())
				}
			}
			if fake.checkoutCalls != 0 {
				t.Fatal("unauthorized provider call")
			}
			if protocol == "acp" {
				for _, invalid := range []struct{ key, body string }{{"quantity", `{"line_items":[{"id":"pro:monthly","quantity":2}]}`}, {"multiple", `{"line_items":[{"id":"pro:monthly"},{"id":"business:monthly"}]}`}} {
					if w := call("POST", base, invalid.key, invalid.body, true, ""); w.Code != http.StatusBadRequest {
						t.Fatalf("unsupported plan quantity: %d %s", w.Code, w.Body.String())
					}
				}
			}
			w := call("POST", base, "first", body, true, "")
			if w.Code != 201 || !strings.Contains(w.Body.String(), `"requires_escalation"`) || !strings.Contains(w.Body.String(), `"continue_url"`) {
				t.Fatalf("create=%d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("cache policy missing")
			}
			w = call("POST", base, "first", body, true, "")
			if w.Code >= 300 || fake.checkoutCalls != 1 {
				t.Fatalf("retry=%d creates=%d %s", w.Code, fake.checkoutCalls, w.Body.String())
			}
			mismatch := strings.ReplaceAll(body, "pro:monthly", "business:monthly")
			w = call("POST", base, "first", mismatch, true, "")
			want := 409
			if protocol == "acp" {
				want = 422
			}
			if w.Code != want {
				t.Fatalf("mismatch=%d want=%d %s", w.Code, want, w.Body.String())
			}
			w = call("POST", base, "other", body, true, "")
			if w.Code != 201 || fake.checkoutCalls != 1 {
				t.Fatalf("shared pending=%d %s", w.Code, w.Body.String())
			}
			// A member of the same team cannot see or mutate its billing session.
			member, err := store.CreateUser(t.Context(), protocol+"-member@example.test", "hash")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.AddTeamMember(t.Context(), buyer.TenantID, member, database.TenantRoleMember, buyer.UserID); err != nil {
				t.Fatal(err)
			}
			if err := store.SetActiveTenantID(t.Context(), member, buyer.TenantID); err != nil {
				t.Fatal(err)
			}
			ownerToken := token
			token, err = auth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, member)
			if err != nil {
				t.Fatal(err)
			}
			if w := call("GET", base+"/cs_test", "", "", true, ""); w.Code != 403 {
				t.Fatalf("member status=%d %s", w.Code, w.Body.String())
			}
			token = ownerToken
			fake.sessions[0].TenantID = uuid.New().String()
			if w := call("GET", base+"/cs_test", "", "", true, ""); w.Code != 404 {
				t.Fatalf("cross-team session=%d %s", w.Code, w.Body.String())
			}
			fake.sessions[0].TenantID = buyer.TenantID.String()
			w = call("POST", base+"/cs_test/complete", "complete", "{}", true, "")
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"requires_escalation"`) {
				t.Fatalf("complete=%d %s", w.Code, w.Body.String())
			}
			fake.sessions[0].Status = "complete"
			fake.sessions[0].PaymentStatus = "unpaid"
			w = call("GET", base+"/cs_test", "", "", true, "")
			if !strings.Contains(w.Body.String(), `"complete_in_progress"`) {
				t.Fatal(w.Body.String())
			}
			fake.sessions[0].PaymentStatus = "paid"
			fake.sessions[0].SubscriptionID = "sub_real"
			w = call("GET", base+"/cs_test", "", "", true, "")
			if !strings.Contains(w.Body.String(), `"completed"`) || !strings.Contains(w.Body.String(), `"sub_real"`) {
				t.Fatal(w.Body.String())
			}
			if w := call("POST", base, "new-after-payment", body, true, ""); w.Code != 409 {
				t.Fatalf("duplicate subscription=%d %s", w.Code, w.Body.String())
			}
			if w := call("POST", base+"/cs_test/cancel", "cancel", "{}", true, ""); w.Code != 405 {
				t.Fatalf("paid cancellation=%d", w.Code)
			}
		})
	}
}

type commerceFollower struct{}

func (commerceFollower) IsLeader() bool        { return false }
func (commerceFollower) GetLeaderAddr() string { return "" }

func TestCommerceUnavailableRailsNeverIssuePaymentProof(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	h.ctx.Config.AIEnabled = true
	buyer, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{Email: "rails@example.test", HashedPassword: "hash", TeamName: "Rails"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, buyer.UserID)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.registerCommerce(mux)
	for _, rail := range []string{"mpp", "x402"} {
		req := httptest.NewRequest(http.MethodPost, "/api/cloud/commerce/"+rail+"/purchases/cs_fake/pay", strings.NewReader(`{}`))
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set("Payment-Signature", "untrusted")
		req.Header.Set("Payment-Authorization", "untrusted")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 501 || !strings.Contains(w.Body.String(), "payment_rail_unavailable") {
			t.Fatalf("rail=%s status=%d %s", rail, w.Code, w.Body.String())
		}
		for _, header := range []string{"WWW-Authenticate", "Payment-Receipt", "Payment-Required", "Payment-Response"} {
			if w.Header().Get(header) != "" {
				t.Fatalf("fabricated %s", header)
			}
		}
	}
	if f := h.stripe.(*fakeStripeClient); f.checkoutCalls != 0 || f.lastCustomerInput != nil {
		t.Fatal("unsupported rail mutated Stripe")
	}
	h.ctx.Cluster = commerceFollower{}
	w := httptest.NewRecorder()
	h.commerceGate(func(http.ResponseWriter, *http.Request) { t.Fatal("follower handler reached") })(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 503 {
		t.Fatalf("follower=%d", w.Code)
	}
}

func TestReuseCheckoutRejectsCompetingTermsAndPendingPayment(t *testing.T) {
	h, f := planCatalogTestHandler()
	teamID := uuid.New()
	f.sessions = []*checkoutSessionOutput{{ID: "cs_pending", TenantID: teamID.String(), Status: "open", PlanCode: "pro", BillingInterval: "monthly", PriceID: "pro_month"}}
	input := createCheckoutSessionInput{CustomerID: "cus_test", TenantID: teamID, PlanCode: "business", BillingInterval: "monthly", PriceID: "business_month"}
	if _, err := h.reuseOrCreateCheckout(t.Context(), input); !errors.Is(err, errCheckoutConflict) {
		t.Fatalf("competing plan=%v", err)
	}
	input.PlanCode = "pro"
	input.PriceID = "changed_price"
	if _, err := h.reuseOrCreateCheckout(t.Context(), input); !errors.Is(err, errCheckoutConflict) {
		t.Fatalf("changed price=%v", err)
	}
	f.sessions[0].Status = "complete"
	f.sessions[0].SubscriptionID = "sub_pending"
	input.PriceID = "pro_month"
	if _, err := h.reuseOrCreateCheckout(t.Context(), input); !errors.Is(err, errCheckoutConflict) {
		t.Fatalf("pending completion=%v", err)
	}
	if f.checkoutCalls != 0 {
		t.Fatal("created competing subscription checkout")
	}
}

func TestCommerceCancelExpiresOnlyThePendingCheckout(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	h.ctx.Config.AIEnabled = true
	buyer, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{Email: "cancel@example.test", HashedPassword: "hash", TeamName: "Cancel"})
	if err != nil {
		t.Fatal(err)
	}
	f := h.stripe.(*fakeStripeClient)
	f.prices = map[string]*stripePriceOutput{"price_pro": {AmountMinor: 1200, Currency: "EUR", Interval: "month", Active: true}}
	call := func(action, id, key, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		req.SetPathValue("id", id)
		req.Header.Set("API-Version", acpVersion)
		req.Header.Set("Idempotency-Key", key)
		req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, buyer.UserID))
		w := httptest.NewRecorder()
		h.handleCommerceSession("acp", action).ServeHTTP(w, req)
		return w
	}
	body := `{"line_items":[{"id":"pro:monthly"}],"currency":"eur","capabilities":{}}`
	if w := call("create", "", "first", body); w.Code != 201 {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	firstKey := f.lastCheckoutInput.IdempotencyKey
	if firstKey == "" {
		t.Fatal("Stripe idempotency key missing")
	}
	if w := call("cancel", "cs_test", "cancel", "{}"); w.Code != 200 || !strings.Contains(w.Body.String(), `"canceled"`) {
		t.Fatalf("cancel=%d %s", w.Code, w.Body.String())
	}
	if w := call("cancel", "cs_test", "cancel", "{}"); w.Code != 200 || w.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("retry cancel=%d %s", w.Code, w.Body.String())
	}
	if w := call("create", "", "second", body); w.Code != 201 {
		t.Fatalf("new create=%d %s", w.Code, w.Body.String())
	}
	if f.checkoutCalls != 2 || f.lastCheckoutInput.IdempotencyKey == firstKey {
		t.Fatal("fresh Checkout did not follow expired prior session")
	}
	account, err := store.GetCloudBillingAccount(t.Context(), buyer.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if account.PlanCode != database.CloudPlanFree || account.StripeSubscriptionID != "" {
		t.Fatalf("cancel changed subscription authority: %+v", account)
	}
}

func TestBillingFinalTeamContextMatchesApproval(t *testing.T) {
	teamID := uuid.New()
	for _, tc := range []struct {
		marker, expected string
		want             bool
	}{{"", "", true}, {"webmcp", teamID.String(), true}, {"webmcp", uuid.New().String(), false}, {"webmcp", "", false}, {"webmcp", "invalid", false}} {
		req := httptest.NewRequest("POST", "/", nil)
		if tc.marker != "" {
			req.Header.Set("X-HitKeep-Agent", tc.marker)
		}
		if tc.expected != "" {
			req.Header.Set("X-HitKeep-Agent-Team", tc.expected)
		}
		w := httptest.NewRecorder()
		if got := shared.CheckAgentTeam(w, req, teamID); got != tc.want {
			t.Fatalf("context match=%v want %v", got, tc.want)
		}
	}
}
