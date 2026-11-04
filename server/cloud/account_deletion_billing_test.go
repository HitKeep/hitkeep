//go:build billing

package cloud

import (
	"context"
	"errors"
	"fmt"
	stripe "github.com/stripe/stripe-go/v87"
	"testing"

	"hitkeep/api"
	"hitkeep/database"
)

func TestAccountDeletionClosedWebhookDoesNotRecreateAuthorityOrBilling(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{Email: "closed-webhook@example.test", HashedPassword: "hash", TeamName: "Closed", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordClosedCloudAccount(ctx, database.ClosedCloudAccount{TeamID: account.TenantID, CustomerID: "cus_closed", SubscriptionID: "sub_closed", CancelAtPeriodEnd: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Exec(ctx, `UPDATE closed_cloud_accounts SET removal_started=TRUE WHERE original_team_id=?`, account.TenantID); err != nil {
		t.Fatal(err)
	}
	event := stripe.Event{ID: "evt_closed_account", Type: "customer.subscription.updated", Data: &stripe.EventData{Raw: []byte(fmt.Sprintf(`{"id":"sub_closed","customer":"cus_closed","status":"active","metadata":{"tenant_id":%q}}`, account.TenantID.String()))}}
	for range 2 {
		if err := h.handleStripeEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.GetCloudBillingAccount(ctx, account.TenantID); !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
		t.Fatalf("closed billing recreated: %v", err)
	}
	var events int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM cloud_billing_events WHERE stripe_event_id=?`, event.ID).Scan(&events); err != nil || events != 0 {
		t.Fatalf("closed event processing=%d err=%v", events, err)
	}
}

type accountClosureStripeStub struct {
	sub                    accountClosureSubscription
	sessions               []*checkoutSessionOutput
	confirm                bool
	cancelErr              error
	cancelCalls, retrieves int
	keys                   []string
}

func (f *accountClosureStripeStub) ListCheckoutSessions(context.Context, string) ([]*checkoutSessionOutput, error) {
	return f.sessions, nil
}
func (f *accountClosureStripeStub) ExpireCheckoutSession(_ context.Context, id, key string) (*checkoutSessionOutput, error) {
	f.keys = append(f.keys, key)
	return &checkoutSessionOutput{ID: id, Status: "expired"}, nil
}
func (f *accountClosureStripeStub) GetAccountClosureSubscription(context.Context, string) (accountClosureSubscription, error) {
	f.retrieves++
	return f.sub, nil
}
func (f *accountClosureStripeStub) StopAccountClosureRenewal(_ context.Context, _ string, key string) error {
	f.cancelCalls++
	f.keys = append(f.keys, key)
	if f.confirm {
		f.sub.CancelAtPeriodEnd = true
	}
	return f.cancelErr
}

func TestAccountDeletionRequiresVerifiedProviderCancellationBeforeReceipt(t *testing.T) {
	_, store := setupCloudTestHandler(t)
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{Email: "close@example.test", HashedPassword: "hash", TeamName: "Closing", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertCloudBillingAccount(ctx, database.CloudBillingAccount{TenantID: account.TenantID, PlanCode: "pro", PlanName: "Pro", SubscriptionStatus: "active", StripeCustomerID: "cus_close", StripeSubscriptionID: "sub_close"}); err != nil {
		t.Fatal(err)
	}
	team := api.AccountDeletionTeam{ID: account.TenantID, Action: "close", MemberCount: 1}
	fake := &accountClosureStripeStub{sub: accountClosureSubscription{ID: "sub_close", CustomerID: "cus_close", Status: "active"}}
	if err := stopAccountDeletionRenewals(ctx, store, fake, team, "attempt1"); !errors.Is(err, database.ErrAccountDeletionUnavailable) {
		t.Fatalf("unconfirmed result=%v", err)
	}
	if closed, err := store.IsClosedCloudAccount(ctx, team.ID, "", ""); err != nil || closed {
		t.Fatalf("unconfirmed receipt=%v err=%v", closed, err)
	}
	if user, err := store.GetUserByID(ctx, account.UserID); err != nil || user == nil {
		t.Fatalf("failed provider removed account: %v %v", user, err)
	}
	fake.confirm = true
	if err := stopAccountDeletionRenewals(ctx, store, fake, team, "attempt1"); err != nil {
		t.Fatal(err)
	}
	if fake.cancelCalls != 2 || fake.keys[0] != fake.keys[1] || fake.retrieves != 4 {
		t.Fatalf("calls=%d keys=%v reads=%d", fake.cancelCalls, fake.keys, fake.retrieves)
	}
	if closed, err := store.IsClosedCloudAccount(ctx, team.ID, "", ""); err != nil || !closed {
		t.Fatalf("verified receipt=%v err=%v", closed, err)
	}
	if err := stopAccountDeletionRenewals(ctx, store, fake, team, "attempt1"); err != nil {
		t.Fatal(err)
	}
	if fake.cancelCalls != 2 {
		t.Fatalf("already canceled subscription changed again")
	}
}
