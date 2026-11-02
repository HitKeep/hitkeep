//go:build billing

package cloud

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	stripe "github.com/stripe/stripe-go/v87"
)

func checkoutSessionFromStripe(s *stripe.CheckoutSession) *checkoutSessionOutput {
	out := &checkoutSessionOutput{PriceID: s.Metadata["price_id"], ID: s.ID, URL: s.URL, Status: string(s.Status), PaymentStatus: string(s.PaymentStatus), TenantID: s.Metadata["tenant_id"], PlanCode: s.Metadata["plan_code"], BillingInterval: s.Metadata["billing_interval"], Currency: string(s.Currency), AmountSubtotal: s.AmountSubtotal, AmountTotal: s.AmountTotal, ExpiresAt: s.ExpiresAt}
	if s.Customer != nil {
		out.CustomerID = s.Customer.ID
	}
	if s.Subscription != nil {
		out.SubscriptionID = s.Subscription.ID
		out.SubscriptionStatus = string(s.Subscription.Status)
	}
	return out
}

func (c *stripeSDKClient) GetCheckoutSession(ctx context.Context, id string) (*checkoutSessionOutput, error) {
	params := &stripe.CheckoutSessionRetrieveParams{}
	params.AddExpand("subscription")
	setStripeVersionHeader(&params.Params)
	s, err := c.client.V1CheckoutSessions.Retrieve(ctx, id, params)
	if err != nil {
		return nil, fmt.Errorf("retrieve checkout: %w", err)
	}
	return checkoutSessionFromStripe(s), nil
}

func (c *stripeSDKClient) ListCheckoutSessions(ctx context.Context, customer string) ([]*checkoutSessionOutput, error) {
	params := &stripe.CheckoutSessionListParams{Customer: new(customer)}
	params.AddExpand("data.subscription")
	params.Limit = new(int64(100))
	sessions := make([]*checkoutSessionOutput, 0)
	for s, err := range c.client.V1CheckoutSessions.List(ctx, params).All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("list checkout: %w", err)
		}
		if len(sessions) >= 1000 {
			return nil, errors.New("checkout history requires operator review")
		}
		sessions = append(sessions, checkoutSessionFromStripe(s))
	}
	return sessions, nil
}

func (c *stripeSDKClient) ExpireCheckoutSession(ctx context.Context, id, key string) (*checkoutSessionOutput, error) {
	params := &stripe.CheckoutSessionExpireParams{}
	setStripeVersionHeader(&params.Params)
	setStripeIdempotencyKey(&params.Params, key)
	s, err := c.client.V1CheckoutSessions.Expire(ctx, id, params)
	if err != nil {
		return nil, fmt.Errorf("expire checkout: %w", err)
	}
	return checkoutSessionFromStripe(s), nil
}

// Stripe owns durable session state; both browser and protocol purchases share it.
func (h *handler) reuseOrCreateCheckout(ctx context.Context, input createCheckoutSessionInput) (*checkoutSessionOutput, error) {
	sessions, err := h.stripe.ListCheckoutSessions(ctx, input.CustomerID)
	if err != nil {
		return nil, err
	}
	previous := "initial"
	var pending *checkoutSessionOutput
	for _, s := range sessions {
		if s.TenantID != input.TenantID.String() {
			continue
		}
		if previous == "initial" {
			previous = s.ID
		}
		// Only Stripe's terminal subscription state makes a historical payment safe to replace.
		if s.Status == "complete" && s.SubscriptionStatus != string(stripe.SubscriptionStatusCanceled) && s.SubscriptionStatus != string(stripe.SubscriptionStatusIncompleteExpired) {
			return nil, errCheckoutConflict
		}
		if s.Status != "open" {
			continue
		}
		if s.PlanCode != input.PlanCode || s.BillingInterval != input.BillingInterval || (s.PriceID != "" && s.PriceID != input.PriceID) {
			return nil, errCheckoutConflict
		}
		if pending != nil && pending.ID != s.ID {
			return nil, errCheckoutConflict
		}
		pending = s
	}
	if pending != nil {
		return pending, nil
	}
	// One key for the next team purchase, independent of browser/protocol and plan.
	input.IdempotencyKey = fmt.Sprintf("checkout_%x", sha256.Sum256([]byte(input.TenantID.String()+":"+previous)))
	return h.stripe.CreateCheckoutSession(ctx, input)
}

var errCheckoutConflict = errors.New("a checkout already exists; finish or cancel it before starting another")
