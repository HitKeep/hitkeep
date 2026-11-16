//go:build billing

package cloud

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go/v87"

	"hitkeep/api"
	"hitkeep/database"
	"hitkeep/server/shared"
)

type accountClosureSubscription struct {
	ID, CustomerID, Status string
	CancelAtPeriodEnd      bool
	CurrentPeriodEnd       int64
}

type accountClosureStripeClient interface {
	ListCheckoutSessions(context.Context, string) ([]*checkoutSessionOutput, error)
	ExpireCheckoutSession(context.Context, string, string) (*checkoutSessionOutput, error)
	GetAccountClosureSubscription(context.Context, string) (accountClosureSubscription, error)
	StopAccountClosureRenewal(context.Context, string, string) error
}

// StopAccountDeletionRenewals is purpose-bound to the lifecycle owner's already
// checked solo team. It neither terminates paid time nor creates charges/refunds.
func StopAccountDeletionRenewals(ctx context.Context, c *shared.Context, team api.AccountDeletionTeam, attemptID string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if c == nil || c.Config == nil || !c.Config.CloudHosted {
		return database.ErrAccountDeletionUnavailable
	}
	if team.CancellationRequired && strings.TrimSpace(c.Config.StripeSecretKey) == "" {
		return database.ErrAccountDeletionUnavailable
	}
	return stopAccountDeletionRenewals(ctx, c.Store, newStripeSDKClient(c.Config.StripeSecretKey), team, attemptID)
}

func stopAccountDeletionRenewals(ctx context.Context, store *database.Store, client accountClosureStripeClient, team api.AccountDeletionTeam, attemptID string) error {
	if team.Action != "close" || team.MemberCount != 1 {
		return database.ErrAccountDeletionBlocked
	}
	account, err := store.GetCloudBillingAccount(ctx, team.ID)
	if errors.Is(err, database.ErrCloudBillingAccountNotFound) {
		return store.RecordClosedCloudAccount(ctx, database.ClosedCloudAccount{TeamID: team.ID, CancelAtPeriodEnd: true})
	}
	if err != nil {
		return database.ErrAccountDeletionUnavailable
	}
	if account.StripeCustomerID == "" {
		if account.StripeSubscriptionID != "" || account.PlanCode != database.CloudPlanFree || !database.CloudSubscriptionStatusIsFree(account.SubscriptionStatus) {
			return database.ErrAccountDeletionUnavailable
		}
		return store.RecordClosedCloudAccount(ctx, database.ClosedCloudAccount{TeamID: team.ID, CancelAtPeriodEnd: true})
	}
	sessions, err := client.ListCheckoutSessions(ctx, account.StripeCustomerID)
	if err != nil {
		return database.ErrAccountDeletionUnavailable
	}
	for _, session := range sessions {
		if session == nil {
			return database.ErrAccountDeletionUnavailable
		}
		if session.TenantID != team.ID.String() {
			if session.Status == "open" {
				return database.ErrAccountDeletionChanged
			}
			continue
		}
		if session.Status == "complete" && session.SubscriptionID != "" && session.SubscriptionID != account.StripeSubscriptionID && session.SubscriptionStatus != "canceled" && session.SubscriptionStatus != "incomplete_expired" {
			return database.ErrAccountDeletionChanged
		}
		if session.Status == "open" {
			expired, err := client.ExpireCheckoutSession(ctx, session.ID, stripeIdempotencyKey("account-close-checkout", team.ID.String(), session.ID))
			if err != nil || expired == nil || expired.Status != "expired" {
				return database.ErrAccountDeletionUnavailable
			}
		}
	}
	receipt := database.ClosedCloudAccount{TeamID: team.ID, CustomerID: account.StripeCustomerID, SubscriptionID: account.StripeSubscriptionID, CancelAtPeriodEnd: true}
	if account.StripeSubscriptionID != "" {
		sub, err := client.GetAccountClosureSubscription(ctx, account.StripeSubscriptionID)
		if err != nil || sub.ID != account.StripeSubscriptionID || sub.CustomerID != account.StripeCustomerID {
			return database.ErrAccountDeletionUnavailable
		}
		terminal := sub.Status == "canceled" || sub.Status == "incomplete_expired"
		if !terminal && !sub.CancelAtPeriodEnd {
			if err := client.StopAccountClosureRenewal(ctx, sub.ID, stripeIdempotencyKey("account-close-renewal", team.ID.String(), sub.ID, attemptID)); err != nil {
				return database.ErrAccountDeletionUnavailable
			}
			// A successful update alone is not the provider-state confirmation.
			sub, err = client.GetAccountClosureSubscription(ctx, account.StripeSubscriptionID)
			if err != nil || sub.ID != account.StripeSubscriptionID || sub.CustomerID != account.StripeCustomerID || !sub.CancelAtPeriodEnd {
				return database.ErrAccountDeletionUnavailable
			}
		}
		if sub.CurrentPeriodEnd > 0 {
			end := time.Unix(sub.CurrentPeriodEnd, 0).UTC()
			receipt.CurrentPeriodEnd = &end
		}
	} else if account.PlanCode != database.CloudPlanFree || !database.CloudSubscriptionStatusIsFree(account.SubscriptionStatus) {
		return database.ErrAccountDeletionUnavailable
	}
	return store.RecordClosedCloudAccount(ctx, receipt)
}

func (c *stripeSDKClient) GetAccountClosureSubscription(ctx context.Context, id string) (accountClosureSubscription, error) {
	params := &stripe.SubscriptionRetrieveParams{}
	setStripeVersionHeader(&params.Params)
	sub, err := c.client.V1Subscriptions.Retrieve(ctx, strings.TrimSpace(id), params)
	if err != nil || sub == nil {
		return accountClosureSubscription{}, fmt.Errorf("subscription verification unavailable")
	}
	result := accountClosureSubscription{ID: sub.ID, CustomerID: stripeCustomerID(sub.Customer), Status: string(sub.Status), CancelAtPeriodEnd: sub.CancelAtPeriodEnd}
	if sub.Items != nil {
		for _, item := range sub.Items.Data {
			if item != nil {
				result.CurrentPeriodEnd = max(result.CurrentPeriodEnd, item.CurrentPeriodEnd)
			}
		}
	}
	return result, nil
}

func (c *stripeSDKClient) StopAccountClosureRenewal(ctx context.Context, id, key string) error {
	params := &stripe.SubscriptionUpdateParams{CancelAtPeriodEnd: new(true), ProrationBehavior: new("none")}
	setStripeVersionHeader(&params.Params)
	setStripeIdempotencyKey(&params.Params, key)
	_, err := c.client.V1Subscriptions.Update(ctx, id, params)
	if err != nil {
		return errors.New("subscription cancellation unavailable")
	}
	return nil
}
