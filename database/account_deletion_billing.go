//go:build billing

package database

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

func (s *Store) accountDeletionBilling(ctx context.Context, teamID uuid.UUID) (string, bool, error) {
	account, err := s.GetCloudBillingAccount(ctx, teamID)
	if errors.Is(err, ErrCloudBillingAccountNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%s\n%s\n%s\n%s\n%s", account.StripeCustomerID, account.StripeSubscriptionID, account.SubscriptionStatus, account.PlanCode, account.StripePriceID))), account.StripeCustomerID != "" || account.StripeSubscriptionID != "", nil
}

// SerializeCloudBillingMutation excludes a new provider checkout throughout
// final account closure, including provider verification before local purge.
func (s *Store) SerializeCloudBillingMutation() func() {
	s.billingLifecycleMu.Lock()
	return s.billingLifecycleMu.Unlock
}
