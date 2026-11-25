//go:build billing

package cloud

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v87"

	"hitkeep/database"
)

const annualCoupon = "HKBF26"

var annualOfferEnd = time.Date(2026, time.December, 7, 8, 0, 0, 0, time.UTC)

type annualOfferPrice struct {
	FirstInvoice int64  `json:"first_invoice_minor"`
	Renewal      int64  `json:"renewal_minor"`
	Currency     string `json:"currency"`
}

type annualOffer struct {
	Code   string                      `json:"code"`
	EndsAt time.Time                   `json:"ends_at"`
	Prices map[string]annualOfferPrice `json:"prices"`
}

func (h *handler) now() time.Time {
	if h.clock != nil {
		return h.clock()
	}
	return time.Now()
}

// Stripe requires at least 30 minutes and at most 24 hours. Leave five seconds for transit.
func annualSessionExpiry(now time.Time) int64 {
	if !now.Before(annualOfferEnd) {
		return 0
	}
	if annualOfferEnd.Sub(now) < 30*time.Minute+5*time.Second {
		return now.Add(30*time.Minute + 5*time.Second).Unix()
	}
	return min(now.Add(24*time.Hour).Unix(), annualOfferEnd.Unix())
}

func (c *stripeSDKClient) GetAnnualCoupon(ctx context.Context) (*stripe.Coupon, error) {
	params := &stripe.CouponRetrieveParams{}
	params.AddExpand("applies_to")
	setStripeVersionHeader(&params.Params)
	coupon, err := c.client.V1Coupons.Retrieve(ctx, annualCoupon, params)
	if e, ok := errors.AsType[*stripe.Error](err); ok && e.Code == stripe.ErrorCodeResourceMissing {
		return nil, nil
	}
	return coupon, err
}

func (h *handler) annualOfferFor(ctx context.Context, teamID uuid.UUID, account *database.CloudBillingAccount, plans ...string) (*annualOffer, error) {
	conf := h.ctx.Config
	region := strings.ToUpper(strings.TrimSpace(conf.CloudJurisdiction))
	if !conf.CloudHosted || conf.StripeSecretKey == "" || (region != "EU" && region != "US") || !h.now().Before(annualOfferEnd) {
		return nil, nil
	}
	if account != nil && (account.PlanCode != database.CloudPlanFree || account.StripeSubscriptionID != "" || (account.SubscriptionStatus != database.CloudSubscriptionStatusFree && account.SubscriptionStatus != subscriptionStatusPending)) {
		return nil, nil
	}
	operator, err := h.ctx.Store.IsTeamOwnedByOperator(ctx, teamID)
	if err != nil || operator {
		return nil, err
	}
	closed, err := h.ctx.Store.IsClosedCloudAccount(ctx, teamID, "", "")
	if err != nil || closed {
		return nil, err
	}
	prior, err := h.ctx.Store.HasCloudSubscriptionHistory(ctx, teamID)
	if err != nil || prior {
		return nil, err
	}
	coupon, err := h.stripe.GetAnnualCoupon(ctx)
	if err != nil || coupon == nil || coupon.ID != annualCoupon || !coupon.Valid || coupon.Duration != stripe.CouponDurationOnce || coupon.PercentOff != 30 || coupon.RedeemBy != annualOfferEnd.Add(35*time.Minute).Unix() || coupon.AppliesTo == nil {
		return nil, err
	}
	pendingPlan := ""
	if account != nil && account.StripeCustomerID != "" {
		sessions, err := h.stripe.ListCheckoutSessions(ctx, account.StripeCustomerID)
		if err != nil {
			return nil, err
		}
		for _, session := range sessions {
			if session.Status == "open" {
				if session.TenantID != teamID.String() || session.BillingInterval != database.CloudBillingIntervalAnnual {
					return nil, nil
				}
				pendingPlan = session.PlanCode
			}
			if session.Status == "complete" && session.SubscriptionID != "" {
				return nil, nil
			}
		}
	}
	currency := "EUR"
	if region == "US" {
		currency = "USD"
	}
	offer := &annualOffer{Code: annualCoupon, EndsAt: annualOfferEnd, Prices: make(map[string]annualOfferPrice)}
	for _, plan := range plans {
		if pendingPlan != "" && plan != pendingPlan {
			continue
		}
		id := priceIDForPlan(conf, plan, database.CloudBillingIntervalAnnual)
		if id == "" {
			continue
		}
		price, err := h.stripe.GetPrice(ctx, id)
		if err != nil {
			return nil, err
		}
		if price == nil || !price.Active || price.AmountMinor <= 0 || price.Interval != "year" || price.IntervalCount != 1 || !strings.EqualFold(price.Currency, currency) || !slices.Contains(coupon.AppliesTo.Products, price.ProductID) {
			continue
		}
		// Stripe rounds the discount to the nearest minor unit before subtraction.
		offer.Prices[plan] = annualOfferPrice{FirstInvoice: price.AmountMinor - (price.AmountMinor*30+50)/100, Renewal: price.AmountMinor, Currency: currency}
	}
	if len(offer.Prices) == 0 {
		return nil, nil
	}
	return offer, nil
}
