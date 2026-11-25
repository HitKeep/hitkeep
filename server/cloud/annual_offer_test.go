//go:build billing

package cloud

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v87"

	"hitkeep/database"
	"hitkeep/server/shared"
)

func TestAnnualOfferSessionExpiry(t *testing.T) {
	for _, test := range []struct{ before, expires time.Duration }{
		{48 * time.Hour, -24 * time.Hour}, {24 * time.Hour, 0}, {31 * time.Minute, 0},
		{30*time.Minute + 5*time.Second, 0}, {30*time.Minute + time.Second, 4 * time.Second}, {30 * time.Minute, 5 * time.Second}, {30*time.Minute - time.Second, 6 * time.Second},
		{time.Second, 30*time.Minute + 4*time.Second},
	} {
		got := annualSessionExpiry(annualOfferEnd.Add(-test.before))
		if got != annualOfferEnd.Add(test.expires).Unix() {
			t.Errorf("%v before cutoff: expiry %v", test.before, time.Unix(got, 0))
		}
	}
	for _, offset := range []time.Duration{0, time.Nanosecond, time.Hour} {
		if annualSessionExpiry(annualOfferEnd.Add(offset)) != 0 {
			t.Errorf("admitted checkout %v after cutoff", offset)
		}
	}
}

func TestAnnualOfferStateAndCheckout(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	t.Cleanup(func() { _ = store.Close() })
	now := annualOfferEnd.Add(-time.Minute)
	h.clock = func() time.Time { return now }
	buyer, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{Email: "offer@example.test", TeamName: "First subscription", HashedPassword: "hash"})
	if err != nil {
		t.Fatal(err)
	}
	f := h.stripe.(*fakeStripeClient)
	f.coupon = &stripe.Coupon{ID: annualCoupon, Valid: true, Duration: stripe.CouponDurationOnce, PercentOff: 30, RedeemBy: annualOfferEnd.Add(35 * time.Minute).Unix(), AppliesTo: &stripe.CouponAppliesTo{Products: []string{"prod_pro", "prod_business"}}}
	f.prices = map[string]*stripePriceOutput{
		"price_pro_annual":      {ProductID: "prod_pro", Active: true, AmountMinor: 15000, Currency: "eur", Interval: "year", IntervalCount: 1},
		"price_business_annual": {ProductID: "prod_business", Active: true, AmountMinor: 39000, Currency: "eur", Interval: "year", IntervalCount: 1},
	}
	checkOffer := func(count int, currency string) {
		t.Helper()
		acct, err := store.GetCloudBillingAccount(t.Context(), buyer.TenantID)
		if err != nil && !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			t.Fatal(err)
		}
		offer, err := h.annualOfferFor(t.Context(), buyer.TenantID, acct, "pro", "business")
		if err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			if offer != nil {
				t.Fatalf("unexpected offer: %+v", offer)
			}
			return
		}
		if offer == nil || offer.Code != annualCoupon || len(offer.Prices) != count || offer.Prices["pro"] != (annualOfferPrice{10500, 15000, currency}) {
			t.Fatalf("wrong regional offer: %+v", offer)
		}
	}
	checkout := func(plan, interval, expected string, team uuid.UUID, status int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/cloud/billing/checkout", nil)
		req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, buyer.UserID))
		w := httptest.NewRecorder()
		h.createBillingCheckout(w, req, billingCheckoutSessionRequest{ExpectedTeamID: team, ExpectedOffer: expected, PlanCode: plan, BillingInterval: interval})
		if w.Code != status {
			t.Fatalf("checkout status %d, want %d: %s", w.Code, status, w.Body)
		}
	}
	checkOffer(2, "EUR")
	h.ctx.Config.CloudJurisdiction = "US"
	checkOffer(0, "")
	for _, p := range f.prices {
		p.Currency = "usd"
	}
	checkOffer(2, "USD")
	h.ctx.Config.CloudJurisdiction = "unconfigured"
	checkOffer(0, "")
	h.ctx.Config.CloudJurisdiction = "US"
	h.ctx.Config.CloudHosted = false
	checkOffer(0, "")
	h.ctx.Config.CloudHosted = true
	f.coupon.Duration = stripe.CouponDurationForever
	checkOffer(0, "")
	f.coupon.Duration = stripe.CouponDurationOnce
	f.prices["price_business_annual"].Interval = "month"
	checkOffer(1, "USD")
	f.prices["price_business_annual"].Interval = "year"
	checkout("pro", "annual", annualCoupon, uuid.New(), http.StatusConflict)
	checkout("pro", "monthly", annualCoupon, buyer.TenantID, http.StatusConflict)
	checkout("pro", "annual", annualCoupon, buyer.TenantID, http.StatusOK)
	if f.lastCheckoutInput.Coupon != annualCoupon || f.lastCheckoutInput.PriceID != "price_pro_annual" || f.lastCheckoutInput.ExpiresAt != annualSessionExpiry(now) {
		t.Fatalf("wrong discount input: %+v", f.lastCheckoutInput)
	}
	checkout("pro", "annual", annualCoupon, buyer.TenantID, http.StatusOK)
	now = annualOfferEnd
	checkOffer(0, "")
	checkout("pro", "annual", annualCoupon, buyer.TenantID, http.StatusConflict)
	checkout("pro", "annual", "", buyer.TenantID, http.StatusConflict)
	if f.checkoutCalls != 1 {
		t.Fatalf("created %d checkouts", f.checkoutCalls)
	}
	now = annualOfferEnd.Add(-time.Hour)
	f.sessions = nil
	if _, err := store.RecordCloudConversionEvent(t.Context(), database.CloudConversionEvent{TenantID: buyer.TenantID, EventName: database.CloudConversionSubscriptionActivated, PlanCode: "business"}); err != nil {
		t.Fatal(err)
	}
	checkOffer(0, "")
	checkout("business", "annual", annualCoupon, buyer.TenantID, http.StatusConflict)
}
