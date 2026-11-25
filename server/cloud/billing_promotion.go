//go:build billing

package cloud

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v87"

	authcore "hitkeep/auth"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

const promotionReceiptPrefix = "hk_promo_"

// Stripe metadata keys are limited to 40 characters.
func promotionReceipt(couponID string) string {
	hash := sha256.Sum256([]byte(couponID))
	return fmt.Sprintf("%s%x", promotionReceiptPrefix, hash[:15])
}

var promotionCodePattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

var errPromotionUnavailable = errors.New("promotion unavailable")

type promotionPrice struct {
	WithCode int64  `json:"with_code_minor"`
	Regular  int64  `json:"regular_minor"`
	Currency string `json:"currency"`
}

type billingPromotion struct {
	couponID   string
	record     *stripe.PromotionCode
	PercentOff float64                              `json:"percent_off"`
	Code       string                               `json:"code"`
	EndsAt     time.Time                            `json:"ends_at"`
	Prices     map[string]map[string]promotionPrice `json:"prices"`
}

// The coupon owns the discount and product scope. Its metadata sets the customer-visible code and redemption deadline.
func promotionFromCoupon(coupon *stripe.Coupon, id string, now time.Time) *billingPromotion {
	if coupon == nil || coupon.ID != id || !coupon.Valid || coupon.Duration != "once" || coupon.MaxRedemptions != 0 || coupon.PercentOff <= 0 || coupon.PercentOff > 100 || coupon.AppliesTo == nil || len(coupon.AppliesTo.Products) == 0 {
		return nil
	}
	code := coupon.Metadata["hitkeep_promotion_code"]
	end, err := time.Parse(time.RFC3339, coupon.Metadata["hitkeep_promotion_ends_at"])
	if err != nil || end.Nanosecond() != 0 || !now.Before(end) || (coupon.RedeemBy != 0 && coupon.RedeemBy < end.Unix()) || !promotionCodePattern.MatchString(code) {
		return nil
	}
	return &billingPromotion{couponID: id, Code: code, EndsAt: end.UTC(), PercentOff: coupon.PercentOff, Prices: make(map[string]map[string]promotionPrice)}
}

func (h *handler) now() time.Time {
	if h.clock != nil {
		return h.clock()
	}
	return time.Now()
}

func (c *stripeSDKClient) GetPromotionCoupon(ctx context.Context, id string) (*stripe.Coupon, error) {
	params := &stripe.CouponRetrieveParams{}
	params.AddExpand("applies_to")
	setStripeVersionHeader(&params.Params)
	coupon, err := c.client.V1Coupons.Retrieve(ctx, id, params)
	if e, ok := errors.AsType[*stripe.Error](err); ok && e.Code == stripe.ErrorCodeResourceMissing {
		return nil, nil
	}
	return coupon, err
}

// Include inactive records. An exhausted customer must never receive a replacement.
func (c *stripeSDKClient) CustomerPromotions(ctx context.Context, params *stripe.PromotionCodeListParams) ([]*stripe.PromotionCode, error) {
	var records []*stripe.PromotionCode
	for code, err := range c.client.V1PromotionCodes.List(ctx, params).All(ctx) {
		if err != nil {
			return nil, err
		}
		records = append(records, code)
	}
	return records, nil
}

func (c *stripeSDKClient) CreateCustomerPromotion(ctx context.Context, customer string, offer *billingPromotion) (*stripe.PromotionCode, error) {
	input := &stripe.PromotionCodeCreateParams{
		Code: new(offer.Code), Customer: new(customer), MaxRedemptions: new(int64(1)), ExpiresAt: new(offer.EndsAt.Unix()),
		Promotion: &stripe.PromotionCodeCreatePromotionParams{Type: new("coupon"), Coupon: new(offer.couponID)},
	}
	setStripeVersionHeader(&input.Params)
	setStripeIdempotencyKey(&input.Params, stripeIdempotencyKey("issue-promotion", offer.couponID, customer))
	return c.client.V1PromotionCodes.Create(ctx, input)
}

func customerRecord(p *stripe.PromotionCode, customer, code string) bool {
	return p != nil && p.ID != "" && strings.EqualFold(p.Code, code) &&
		p.Customer != nil && p.Customer.ID == customer && p.MaxRedemptions == 1 &&
		p.Promotion != nil && p.Promotion.Type == "coupon" && p.Promotion.Coupon != nil && p.Promotion.Coupon.ID != ""
}

func (offer *billingPromotion) usable(p *stripe.PromotionCode, customer string) bool {
	return customerRecord(p, customer, offer.Code) && p.Promotion.Coupon.ID == offer.couponID && p.ExpiresAt == offer.EndsAt.Unix() && p.Active && p.TimesRedeemed == 0 &&
		(p.Restrictions == nil || (!p.Restrictions.FirstTimeTransaction && p.Restrictions.MinimumAmount == 0 && len(p.Restrictions.CurrencyOptions) == 0))
}

func sameBillingSubscription(sub *stripe.Subscription, a *database.CloudBillingAccount) bool {
	return sub != nil && sub.ID == a.StripeSubscriptionID && stripeCustomerID(sub.Customer) == a.StripeCustomerID &&
		(sub.Metadata["tenant_id"] == "" || sub.Metadata["tenant_id"] == a.TenantID.String())
}

func (c *stripeSDKClient) PromotionCustomer(ctx context.Context, id string) (*stripe.Customer, error) {
	params := &stripe.CustomerRetrieveParams{}
	setStripeVersionHeader(&params.Params)
	return c.client.V1Customers.Retrieve(ctx, id, params)
}

func (c *stripeSDKClient) PromotionSubscription(ctx context.Context, id string) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionRetrieveParams{}
	params.AddExpand("discounts")
	setStripeVersionHeader(&params.Params)
	return c.client.V1Subscriptions.Retrieve(ctx, id, params)
}

func (c *stripeSDKClient) ApplyPromotion(ctx context.Context, sub, customer string, p *stripe.PromotionCode) error {
	promo := p.ID
	params := &stripe.SubscriptionUpdateParams{Discounts: []*stripe.SubscriptionUpdateDiscountParams{{PromotionCode: new(promo)}}, ProrationBehavior: new("none")}
	// Store the Stripe redemption ID on this subscription in the same update as the discount.
	params.AddMetadata(promotionReceipt(p.Promotion.Coupon.ID), promo)
	params.AddExpand("discounts")
	setStripeVersionHeader(&params.Params)
	setStripeIdempotencyKey(&params.Params, stripeIdempotencyKey("redeem-promotion", promo, sub))
	updated, err := c.client.V1Subscriptions.Update(ctx, sub, params)
	if err != nil {
		return err
	}
	if updated == nil || updated.ID != sub || stripeCustomerID(updated.Customer) != customer || len(updated.Discounts) != 1 || updated.Discounts[0] == nil || updated.Discounts[0].PromotionCode == nil || updated.Discounts[0].PromotionCode.ID != promo {
		return errPromotionUnavailable
	}
	return nil
}

func (h *handler) customerCode(ctx context.Context, offer *billingPromotion, customer string) (*stripe.PromotionCode, error) {
	if offer == nil || !h.now().Before(offer.EndsAt) {
		return nil, nil
	}
	p := offer.record
	if p == nil {
		var err error
		p, err = h.stripe.CreateCustomerPromotion(ctx, customer, offer)
		if err != nil {
			return nil, err
		}
	}
	if !offer.usable(p, customer) || !h.now().Before(offer.EndsAt) {
		return nil, nil
	}
	return p, nil
}

func (h *handler) promotionFor(ctx context.Context, team uuid.UUID, account *database.CloudBillingAccount, plan, interval string) (*billingPromotion, error) {
	conf := h.ctx.Config
	if conf.StripePromotionCouponID == "" || !conf.CloudHosted || conf.StripeSecretKey == "" {
		return nil, nil
	}
	var currency string
	switch strings.ToUpper(strings.TrimSpace(conf.CloudJurisdiction)) {
	case "EU", "EU-CENTRAL-1":
		currency = "EUR"
	case "US", "US-EAST-1":
		currency = "USD"
	default:
		return nil, nil
	}
	operator, err := h.ctx.Store.IsTeamOwnedByOperator(ctx, team)
	if err != nil || operator {
		return nil, err
	}
	closed, err := h.ctx.Store.IsClosedCloudAccount(ctx, team, "", "")
	if err != nil || closed {
		return nil, err
	}
	coupon, err := h.stripe.GetPromotionCoupon(ctx, conf.StripePromotionCouponID)
	offer := promotionFromCoupon(coupon, conf.StripePromotionCouponID, h.now())
	if err != nil || offer == nil {
		return nil, err
	}
	if account != nil && account.StripeCustomerID != "" {
		customer, err := h.stripe.PromotionCustomer(ctx, account.StripeCustomerID)
		if err != nil || customer == nil || customer.ID != account.StripeCustomerID || customer.Deleted || customer.Discount != nil {
			return nil, err
		}
		records, err := h.stripe.CustomerPromotions(ctx, &stripe.PromotionCodeListParams{Customer: new(customer.ID), Coupon: new(offer.couponID)})
		if err != nil || len(records) > 1 {
			return nil, err
		}
		if len(records) == 1 {
			if !offer.usable(records[0], customer.ID) {
				return nil, nil
			}
			offer.record = records[0]
		}
	}
	if account != nil && account.StripeSubscriptionID != "" {
		if (account.PlanCode != "pro" && account.PlanCode != "business") || account.StripeCustomerID == "" {
			return nil, nil
		}
		sub, err := h.stripe.PromotionSubscription(ctx, account.StripeSubscriptionID)
		if err != nil {
			return nil, err
		}
		if !offer.addSubscriptionPrice(sub, account, coupon, currency) {
			return nil, nil
		}
		claimed, err := h.subscriptionRedeemed(ctx, sub, offer.Code)
		if err != nil || claimed {
			return nil, err
		}
		return offer, nil
	}
	if account != nil && account.PlanCode != database.CloudPlanFree {
		return nil, nil
	}
	for _, code := range []string{"pro", "business"} {
		for _, cadence := range []string{"monthly", "annual"} {
			if (plan != "" && code != plan) || (interval != "" && cadence != interval) {
				continue
			}
			id := priceIDForPlan(conf, code, cadence)
			if id == "" {
				continue
			}
			price, err := h.stripe.GetPrice(ctx, id)
			if err != nil {
				return nil, err
			}
			if price != nil && price.Active && price.AmountMinor > 0 && price.IntervalCount == 1 && price.Interval == map[string]string{"monthly": "month", "annual": "year"}[cadence] && strings.EqualFold(price.Currency, currency) && slices.Contains(coupon.AppliesTo.Products, price.ProductID) {
				offer.addPrice(code, cadence, price.AmountMinor, currency)
			}
		}
	}
	if len(offer.Prices) == 0 {
		return nil, nil
	}
	return offer, nil
}

func (offer *billingPromotion) addSubscriptionPrice(sub *stripe.Subscription, account *database.CloudBillingAccount, coupon *stripe.Coupon, currency string) bool {
	if !sameBillingSubscription(sub, account) || sub.Status != "active" || sub.CancelAtPeriodEnd || sub.CancelAt != 0 || sub.PauseCollection != nil || sub.Schedule != nil || sub.PendingUpdate != nil || len(sub.Discounts) != 0 {
		return false
	}
	if sub.Items == nil || sub.Items.HasMore || len(sub.Items.Data) != 1 {
		return false
	}
	item := sub.Items.Data[0]
	if item == nil || len(item.Discounts) != 0 || item.Quantity != 1 || item.Price == nil || item.Price.Recurring == nil || item.Price.Product == nil {
		return false
	}
	if !slices.Contains(coupon.AppliesTo.Products, item.Price.Product.ID) || !strings.EqualFold(string(item.Price.Currency), currency) || item.Price.Recurring.IntervalCount != 1 || item.Price.UnitAmount <= 0 {
		return false
	}
	cadence := map[stripe.PriceRecurringInterval]string{"month": "monthly", "year": "annual"}[item.Price.Recurring.Interval]
	if cadence == "" {
		return false
	}
	offer.addPrice(account.PlanCode, cadence, item.Price.UnitAmount, currency)
	return true
}

func (p *billingPromotion) addPrice(plan, interval string, amount int64, currency string) {
	if p.Prices[plan] == nil {
		p.Prices[plan] = make(map[string]promotionPrice)
	}
	p.Prices[plan][interval] = promotionPrice{WithCode: amount - (amount*int64(math.Round(p.PercentOff*100))+5000)/10000, Regular: amount, Currency: currency}
}

func (h *handler) handleRedeemPromotion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TeamID uuid.UUID `json:"team_id"`
			Code   string    `json:"code"`
		}
		if err := json.UnmarshalRead(http.MaxBytesReader(w, r.Body, 4096), &req); err != nil || req.TeamID == uuid.Nil || !promotionCodePattern.MatchString(strings.TrimSpace(req.Code)) {
			http.Error(w, "Invalid promotion", http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		if !h.ctx.Config.CloudHosted || h.ctx.Store == nil || (h.ctx.Cluster != nil && !h.ctx.Cluster.IsLeader()) {
			http.Error(w, "Unavailable", http.StatusServiceUnavailable)
			return
		}
		release := h.ctx.Store.SerializeCloudBillingMutation()
		defer release()
		user := shared.GetUserIDFromContext(r)
		team, err := h.ctx.Store.GetActiveTenantID(ctx, user)
		if err != nil || team != req.TeamID {
			http.Error(w, "Active team changed", http.StatusConflict)
			return
		}
		if !shared.CheckAgentTeam(w, r, team) {
			return
		}
		role, err := h.ctx.Store.GetTenantRole(ctx, team, user)
		if err != nil || !authcore.TeamRoleHasCapability(role, authcore.CapTeamManageSettings) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
		account, err := h.ctx.Store.GetCloudBillingAccount(ctx, team)
		if err != nil || account == nil || account.StripeSubscriptionID == "" || account.StripeCustomerID == "" {
			http.Error(w, "Active subscription required", http.StatusConflict)
			return
		}
		code := strings.TrimSpace(req.Code)
		// A retry must keep its original outcome even if a new coupon now advertises the same code.
		sub, err := h.stripe.PromotionSubscription(ctx, account.StripeSubscriptionID)
		if err != nil {
			http.Error(w, "Unable to confirm subscription", http.StatusBadGateway)
			return
		}
		if !sameBillingSubscription(sub, account) {
			http.Error(w, "Active subscription required", http.StatusConflict)
			return
		}
		applied, err := h.subscriptionRedeemed(ctx, sub, code)
		if err != nil {
			http.Error(w, "Unable to confirm promotion", http.StatusBadGateway)
			return
		}
		if applied {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		offer, err := h.promotionFor(ctx, team, account, "", "")
		if err != nil {
			http.Error(w, "Unable to verify promotion", http.StatusBadGateway)
			return
		}
		if offer == nil {
			http.Error(w, "Promotion unavailable", http.StatusConflict)
			return
		}
		if !strings.EqualFold(code, offer.Code) {
			http.Error(w, "Invalid promotion", http.StatusBadRequest)
			return
		}
		customer := account.StripeCustomerID
		p, err := h.customerCode(ctx, offer, customer)
		if err != nil {
			http.Error(w, "Unable to verify promotion", http.StatusBadGateway)
			return
		}
		if p == nil {
			http.Error(w, "Promotion unavailable", http.StatusConflict)
			return
		}
		if err := h.stripe.ApplyPromotion(ctx, account.StripeSubscriptionID, customer, p); err != nil {
			http.Error(w, "Unable to confirm promotion", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Match a customer-scoped redemption against the receipt stored on this subscription.
func (h *handler) subscriptionRedeemed(ctx context.Context, sub *stripe.Subscription, code string) (bool, error) {
	if !slices.ContainsFunc(slices.Collect(maps.Keys(sub.Metadata)), func(key string) bool { return strings.HasPrefix(key, promotionReceiptPrefix) }) {
		return false, nil
	}
	records, err := h.stripe.CustomerPromotions(ctx, &stripe.PromotionCodeListParams{Customer: new(sub.Customer.ID), Code: new(code)})
	if err != nil {
		return false, err
	}
	for _, p := range records {
		if customerRecord(p, sub.Customer.ID, code) && p.TimesRedeemed == 1 && sub.Metadata[promotionReceipt(p.Promotion.Coupon.ID)] == p.ID {
			return true, nil
		}
	}
	return false, nil
}
