//go:build billing

package cloud

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v87"

	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

const promotionCode = "HKBF26"
const fixtureCoupon = "coupon_test"

var promotionEnd = time.Date(2026, time.December, 7, 8, 0, 0, 0, time.UTC)

func TestBillingPromotion(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	t.Cleanup(func() { _ = store.Close() })
	now := promotionEnd.Add(-time.Second)
	h.clock = func() time.Time { return now }
	ctx := t.Context()
	buyer, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{Email: "promo@example.test", TeamName: "Promo fixture", HashedPassword: "hash"})
	if err != nil {
		t.Fatal(err)
	}
	f := h.stripe.(*fakeStripeClient)
	h.ctx.Config.StripePromotionCouponID = fixtureCoupon
	f.coupon = &stripe.Coupon{ID: fixtureCoupon, Metadata: map[string]string{"hitkeep_promotion_code": promotionCode, "hitkeep_promotion_ends_at": promotionEnd.Format(time.RFC3339)}, Valid: true, Duration: "once", PercentOff: 30, RedeemBy: promotionEnd.Add(35 * time.Minute).Unix(), AppliesTo: &stripe.CouponAppliesTo{Products: []string{"prod_pro", "prod_business"}}}
	f.customer = &stripe.Customer{ID: "cus_test"}
	f.prices = map[string]*stripePriceOutput{}
	quotes := []struct {
		plan, cadence, interval string
		regular, discounted     int64
	}{
		{"pro", "monthly", "month", 1500, 1050},
		{"pro", "annual", "year", 15000, 10500},
		{"business", "monthly", "month", 3900, 2730},
		{"business", "annual", "year", 39000, 27300},
	}
	for _, q := range quotes {
		id := priceIDForPlan(h.ctx.Config, q.plan, q.cadence)
		f.prices[id] = &stripePriceOutput{ProductID: "prod_" + q.plan, Active: true, AmountMinor: q.regular, Currency: "eur", Interval: q.interval, IntervalCount: 1}
	}
	current := func() *database.CloudBillingAccount {
		a, e := store.GetCloudBillingAccount(ctx, buyer.TenantID)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	checkout := func(plan, billing, expected string, team uuid.UUID) int {
		r := httptest.NewRequest(http.MethodPost, "/api/cloud/billing/checkout", nil)
		r = r.WithContext(context.WithValue(r.Context(), shared.UserIDKey, buyer.UserID))
		w := httptest.NewRecorder()
		h.createBillingCheckout(w, r, billingCheckoutSessionRequest{ExpectedTeamID: team, ExpectedPromotion: expected, PlanCode: plan, BillingInterval: billing})
		return w.Code
	}
	// Disabled or incomplete configuration never advertises a discount.
	h.ctx.Config.StripePromotionCouponID = ""
	if offer, e := h.promotionFor(ctx, buyer.TenantID, nil, "", ""); e != nil || offer != nil {
		t.Fatal("advertised unconfigured promotion")
	}
	h.ctx.Config.StripePromotionCouponID = fixtureCoupon
	delete(f.coupon.Metadata, "hitkeep_promotion_ends_at")
	if offer, e := h.promotionFor(ctx, buyer.TenantID, nil, "", ""); e != nil || offer != nil {
		t.Fatal("used the coupon backstop as a public deadline")
	}
	f.coupon.Metadata["hitkeep_promotion_ends_at"] = promotionEnd.Format(time.RFC3339)
	f.coupon.Metadata["hitkeep_promotion_code"] = "SECOND-CODE"
	f.coupon.PercentOff = 17.5
	if offer, e := h.promotionFor(ctx, buyer.TenantID, nil, "pro", "monthly"); e != nil || offer == nil || offer.Code != "SECOND-CODE" || offer.PercentOff != 17.5 || offer.Prices["pro"]["monthly"].WithCode != 1237 {
		t.Fatalf("terms did not come from Stripe: %+v %v", offer, e)
	}
	f.coupon.Metadata["hitkeep_promotion_code"] = promotionCode
	f.coupon.PercentOff = 30
	getState := func(userID uuid.UUID) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(context.WithValue(ctx, shared.UserIDKey, userID), http.MethodGet, "/api/cloud/billing/state", nil)
		w := httptest.NewRecorder()
		h.handleGetBillingState().ServeHTTP(w, req)
		return w
	}
	setRegion := func(region, currency string) {
		h.ctx.Config.CloudJurisdiction = region
		for _, p := range f.prices {
			p.Currency = currency
		}
	}
	for _, tc := range []struct {
		region, currency string
		eligible         bool
	}{
		{"", "EUR", false}, {"ap-southeast-1", "EUR", false},
		{"usual", "USD", false}, {"eucalyptus", "EUR", false},
		{"EU", "EUR", true}, {"eu-central-1", "EUR", true},
		{"US", "USD", true}, {"us-east-1", "USD", true},
		{"  eU  ", "EUR", true}, {" Us-East-1 ", "USD", true},
		{"us-west-2", "USD", false}, {"eu-west-1", "EUR", false},
	} {
		t.Run("billing_state/"+tc.region, func(t *testing.T) {
			setRegion(tc.region, tc.currency)
			w := getState(buyer.UserID)
			var state billingStateResponse
			if w.Code != http.StatusOK {
				t.Fatalf("billing state HTTP %d: %s", w.Code, w.Body.String())
			}
			if err := json.UnmarshalRead(w.Body, &state); err != nil {
				t.Fatal(err)
			}
			if state.TeamID != buyer.TenantID || state.PlanCode != database.CloudPlanFree || state.SubscriptionStatus != database.CloudSubscriptionStatusFree {
				t.Fatalf("unexpected billing state: %+v", state)
			}
			if !tc.eligible {
				if state.Promotion != nil {
					t.Fatalf("unexpected offer: %+v", state.Promotion)
				}
				return
			}
			if state.Promotion == nil || state.Promotion.Code != promotionCode {
				t.Fatalf("missing promotion %q: %+v", promotionCode, state.Promotion)
			}
			for _, q := range quotes {
				got := state.Promotion.Prices[q.plan][q.cadence]
				if got != (promotionPrice{q.discounted, q.regular, tc.currency}) {
					t.Errorf("%s/%s quote: %+v", q.plan, q.cadence, got)
				}
			}
		})
	}
	setRegion("US", "USD")
	if w := getState(uuid.Nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous billing state: %d", w.Code)
	}
	member, err := store.CreateUser(ctx, "region-member@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTeamMember(ctx, buyer.TenantID, member, database.TenantRoleMember, buyer.UserID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveTenantID(ctx, member, buyer.TenantID); err != nil {
		t.Fatal(err)
	}
	if w := getState(member); w.Code != http.StatusForbidden {
		t.Fatalf("member billing state: %d", w.Code)
	}
	if _, err := store.RecordCloudConversionEvent(ctx, database.CloudConversionEvent{TenantID: buyer.TenantID, EventName: database.CloudConversionSubscriptionActivated, PlanCode: "pro"}); err != nil {
		t.Fatal(err)
	}
	f.promotionErr = errors.New("Stripe promotion API unavailable")
	if checkout("business", "annual", promotionCode, buyer.TenantID) != 502 || f.checkoutCalls != 0 {
		t.Fatal("expected promotion silently charged full price")
	}
	if checkout("business", "annual", "", buyer.TenantID) != 200 || f.lastCheckoutInput.AllowPromotionCodes {
		t.Fatal("optional promotion disabled standard checkout")
	}
	f.sessions = nil
	f.checkoutCalls = 0
	f.promotionErr = nil
	if checkout("pro", "monthly", "OBSOLETE", buyer.TenantID) != 409 || f.minted != 0 || f.checkoutCalls != 0 {
		t.Fatal("provisioned a code or session for a stale review")
	}
	if checkout("pro", "monthly", promotionCode, uuid.New()) != 409 || checkout("pro", "monthly", promotionCode, buyer.TenantID) != 200 || !f.lastCheckoutInput.AllowPromotionCodes || f.minted != 1 {
		t.Fatal("manual monthly checkout unavailable")
	}
	if checkout("pro", "monthly", promotionCode, buyer.TenantID) != 200 || f.checkoutCalls != 1 || f.minted != 1 {
		t.Fatal("repeated checkout minted a new code/session")
	}
	f.afterCheckoutList = func() { now = promotionEnd }
	if checkout("pro", "monthly", promotionCode, buyer.TenantID) != 409 {
		t.Fatal("returned explicit checkout after cutoff during lookup")
	}
	f.afterCheckoutList = nil
	if checkout("pro", "monthly", "", buyer.TenantID) != 200 || f.checkoutCalls != 2 || f.sessions[1].Status != "expired" || f.sessions[0].AllowPromotionCodes {
		t.Fatal("ordinary checkout must replace the superseded promotion session at cutoff")
	}
	now = promotionEnd
	if checkout("pro", "monthly", promotionCode, buyer.TenantID) != 409 {
		t.Fatal("accepted stale promotion")
	}
	now = promotionEnd.Add(-time.Second)
	a := current()
	a.PlanCode = "pro"
	a.PlanName = "Pro"
	a.SubscriptionStatus = "active"
	a.BillingInterval = "monthly"
	a.StripeSubscriptionID = "sub_fixture"
	if err := store.UpsertCloudBillingAccount(ctx, *a); err != nil {
		t.Fatal(err)
	}
	item := &stripe.SubscriptionItem{Quantity: 1, Price: &stripe.Price{ID: "price_pro", UnitAmount: 1500, Currency: "usd", Product: &stripe.Product{ID: "prod_pro"}, Recurring: &stripe.PriceRecurring{Interval: "month", IntervalCount: 1}}}
	f.subscription = &stripe.Subscription{ID: a.StripeSubscriptionID, Customer: f.customer, Status: "active", Items: &stripe.SubscriptionItemList{Data: []*stripe.SubscriptionItem{item}}}
	redeem := func(team uuid.UUID, code string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/cloud/billing/promotion", strings.NewReader(fmt.Sprintf(`{"team_id":%q,"code":%q}`, team.String(), code)))
		r = r.WithContext(context.WithValue(r.Context(), shared.UserIDKey, buyer.UserID))
		w := httptest.NewRecorder()
		h.handleRedeemPromotion()(w, r)
		return w.Code
	}
	if redeem(buyer.TenantID, "OTHER") != 400 || redeem(uuid.New(), promotionCode) != 409 {
		t.Fatal("invalid redemption")
	}
	for _, level := range []string{"item", "subscription", "customer"} {
		d := &stripe.Discount{ID: "di_existing"}
		switch level {
		case "item":
			item.Discounts = []*stripe.Discount{d}
		case "subscription":
			f.subscription.Discounts = []*stripe.Discount{d}
		case "customer":
			f.customer.Discount = d
		}
		if redeem(buyer.TenantID, promotionCode) != 409 || f.applied != 0 {
			t.Fatalf("replaced %s discount", level)
		}
		item.Discounts = nil
		f.subscription.Discounts = nil
		f.customer.Discount = nil
	}
	f.subscription.Customer = &stripe.Customer{ID: "wrong_customer"}
	if redeem(buyer.TenantID, promotionCode) != 409 {
		t.Fatal("cross-customer redemption")
	}
	f.subscription.Customer = f.customer
	f.lostApply = true
	if redeem(buyer.TenantID, promotionCode) != 502 {
		t.Fatal("unconfirmed write reported success")
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if got := redeem(buyer.TenantID, " hkbf26 "); got != 204 {
				t.Errorf("concurrent redeem=%d", got)
			}
		})
	}
	wg.Wait()
	now = promotionEnd
	if redeem(buyer.TenantID, promotionCode) != 204 || f.applied != 1 || f.minted != 1 || f.checkoutCalls != 2 {
		t.Fatal("lost response/retry caused another redemption, subscription, or code")
	}
	f.subscription.Discounts = nil
	if redeem(buyer.TenantID, promotionCode) != 204 {
		t.Fatal("lost exact subscription receipt after invoice")
	}
	now = promotionEnd.Add(-time.Minute)
	f.coupon.Metadata["hitkeep_promotion_code"] = "RENAMED"
	if redeem(buyer.TenantID, "RENAMED") != 409 || redeem(buyer.TenantID, promotionCode) != 204 || f.minted != 1 || f.applied != 1 {
		t.Fatal("renamed coupon lost a receipt or granted a second redemption")
	}
	originalCoupon := f.coupon
	next := *originalCoupon
	next.ID = "coupon_next"
	next.Metadata = map[string]string{"hitkeep_promotion_code": "NEXT", "hitkeep_promotion_ends_at": promotionEnd.Format(time.RFC3339)}
	f.coupon = &next
	for _, id := range []string{"", next.ID} {
		h.ctx.Config.StripePromotionCouponID = id
		if redeem(buyer.TenantID, promotionCode) != 204 || f.minted != 1 || f.applied != 1 {
			t.Fatal("configuration change lost the exact subscription receipt")
		}
	}
	next.Metadata["hitkeep_promotion_code"] = promotionCode
	if offer, err := h.promotionFor(ctx, buyer.TenantID, a, "", ""); err != nil || offer != nil {
		t.Fatal("advertised a previously redeemed alias after coupon rotation")
	}
	if redeem(buyer.TenantID, promotionCode) != 204 || f.minted != 1 || f.applied != 1 {
		t.Fatal("retry redeemed a different coupon with the same public code")
	}
	next.Metadata["hitkeep_promotion_code"] = "NEXT"
	if redeem(buyer.TenantID, "NEXT") != 204 || f.minted != 2 || f.applied != 2 {
		t.Fatal("new coupon unavailable")
	}
	f.subscription.Discounts = nil
	if redeem(buyer.TenantID, promotionCode) != 204 || len(f.subscription.Metadata) != 2 || f.applied != 2 {
		t.Fatal("new campaign overwrote the first receipt")
	}
	f.coupon = originalCoupon
	h.ctx.Config.StripePromotionCouponID = fixtureCoupon
	now = promotionEnd
	f.subscription.Metadata["tenant_id"] = uuid.NewString()
	if redeem(buyer.TenantID, promotionCode) != 409 {
		t.Fatal("acknowledged a different team")
	}
	delete(f.subscription.Metadata, "tenant_id")
	f.subscription.Customer = &stripe.Customer{ID: "cus_other"}
	if redeem(buyer.TenantID, promotionCode) != 409 {
		t.Fatal("acknowledged a different customer")
	}
	f.subscription.Customer = f.customer
	f.subscription.ID = "sub_other"
	if redeem(buyer.TenantID, promotionCode) != 409 {
		t.Fatal("acknowledged a different subscription")
	}
	f.subscription.ID = a.StripeSubscriptionID
	delete(f.subscription.Metadata, promotionReceipt(fixtureCoupon))
	if redeem(buyer.TenantID, promotionCode) != 409 {
		t.Fatal("acknowledged exhausted record without subscription evidence")
	}
	now = promotionEnd.Add(-time.Minute)
	if redeem(buyer.TenantID, promotionCode) != 409 || f.minted != 2 {
		t.Fatal("reactivated consumed customer code")
	}
}

func TestCustomerPromotionWire(t *testing.T) {
	var created, consumed bool
	var creates, updates int
	record := func() string {
		return fmt.Sprintf(`{"id":"promo_fixture","code":"HKBF26","customer":"cus_test","promotion":{"type":"coupon","coupon":"coupon_test"},"active":%t,"max_redemptions":1,"times_redeemed":%d,"expires_at":1796630400}`, !consumed, map[bool]int{true: 1}[consumed])
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/promotion_codes":
			q := r.URL.Query()
			if q.Get("customer") != "cus_test" || q.Get("coupon") != fixtureCoupon || q.Has("code") || q.Has("active") {
				t.Errorf("unsafe lookup %v", q)
			}
			data := ""
			if created {
				data = record()
			}
			_, _ = fmt.Fprintf(w, `{"object":"list","data":[%s],"has_more":false}`, data)
		case "POST /v1/promotion_codes":
			_ = r.ParseForm()
			p := r.Form
			if p.Get("customer") != "cus_test" || p.Get("code") != promotionCode || p.Get("max_redemptions") != "1" || p.Get("expires_at") != "1796630400" || p.Get("promotion[coupon]") != fixtureCoupon || p.Get("promotion[type]") != "coupon" || p.Has("restrictions[first_time_transaction]") || r.Header.Get("Idempotency-Key") != stripeIdempotencyKey("issue-promotion", fixtureCoupon, "cus_test") {
				t.Errorf("unsafe issuance %v", p)
			}
			created = true
			creates++
			_, _ = fmt.Fprint(w, record())
		case "POST /v1/subscriptions/sub_fixture":
			_ = r.ParseForm()
			if len(r.Form) != 4 || r.Form.Get("metadata["+promotionReceipt(fixtureCoupon)+"]") != "promo_fixture" || r.Form.Get("discounts[0][promotion_code]") != "promo_fixture" || r.Form.Get("proration_behavior") != "none" || r.Form.Get("expand[0]") != "discounts" || r.Header.Get("Idempotency-Key") == "" {
				t.Errorf("unsafe subscription update: %v", r.Form)
			}
			consumed = true
			updates++
			_, _ = fmt.Fprint(w, `{"id":"sub_fixture","customer":"cus_test","discounts":[{"id":"di_test","promotion_code":"promo_fixture"}]}`)
		default:
			t.Errorf("unexpected route %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	back := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: new(server.URL), HTTPClient: server.Client(), MaxNetworkRetries: new(int64(0))})
	client := &stripeSDKClient{client: stripe.NewClient("sk_fixture", stripe.WithBackends(&stripe.Backends{API: back}))}
	offer := &billingPromotion{Code: promotionCode, EndsAt: promotionEnd, couponID: fixtureCoupon}
	query := &stripe.PromotionCodeListParams{Coupon: new(fixtureCoupon), Customer: new("cus_test")}
	records, e := client.CustomerPromotions(t.Context(), query)
	if e != nil || len(records) != 0 {
		t.Fatalf("unexpected existing promotion %v %v", records, e)
	}
	p, e := client.CreateCustomerPromotion(t.Context(), "cus_test", offer)
	if e != nil || !offer.usable(p, "cus_test") {
		t.Fatalf("issue: %+v %v", p, e)
	}
	records, e = client.CustomerPromotions(t.Context(), query)
	if e != nil || len(records) != 1 || !offer.usable(records[0], "cus_test") {
		t.Fatalf("find: %+v %v", records, e)
	}
	if err := client.ApplyPromotion(t.Context(), "sub_fixture", "cus_test", p); err != nil {
		t.Fatal(err)
	}
	records, e = client.CustomerPromotions(t.Context(), query)
	if e != nil || len(records) != 1 || offer.usable(records[0], "cus_test") || creates != 1 || updates != 1 {
		t.Fatalf("consumed code was reissued: %+v, %v", p, e)
	}
}
