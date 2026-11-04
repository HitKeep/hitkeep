//go:build billing

package database

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

func TestSoloCloudClosureSerializesConcurrentMembershipChange(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	account, err := s.CreateManagedCloudAccount(ctx, CreateManagedCloudAccountInput{Email: "racing-close@example.test", HashedPassword: "hash", TeamName: "Solo", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateManagedCloudAccount(ctx, CreateManagedCloudAccountInput{Email: "racing-member@example.test", HashedPassword: "hash", TeamName: "Other", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	choices := api.AccountDeletionChoices{CloseTeamIDs: []uuid.UUID{account.TenantID}}
	plan, err := s.PlanAccountDeletion(ctx, account.UserID, choices, true)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := AccountDeletionConsequenceHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := s.SaveAccountDeletionIntent(ctx, AccountDeletionIntent{AttemptID: id, UserID: account.UserID, Choices: choices, Plan: plan, ConsequenceHash: hash, Cloud: true, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	m := NewTenantStoreManager(s, t.TempDir())
	t.Cleanup(func() { _ = m.Close() })
	entered := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	deleted := make(chan error, 1)
	go func() {
		deleted <- m.DeleteCurrentAccount(ctx, account.UserID, id, "jti", hash, func() bool { return true }, func(ctx context.Context, team api.AccountDeletionTeam, _ string) error {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return s.RecordClosedCloudAccount(ctx, ClosedCloudAccount{TeamID: team.ID, CancelAtPeriodEnd: true})
		})
	}()
	select {
	case <-entered:
	case err := <-deleted:
		t.Fatalf("closure failed before boundary: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("closure did not reach billing boundary")
	}
	joined := make(chan error, 1)
	go func() {
		joined <- s.AddTeamMember(ctx, account.TenantID, other.UserID, TenantRoleMember, account.UserID)
	}()
	select {
	case err := <-joined:
		t.Fatalf("membership changed during final ownership boundary: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	unblock()
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	if err := <-joined; !errors.Is(err, ErrAccountDeletionBlocked) {
		t.Fatalf("closed team accepted new member: %v", err)
	}
	if user, err := s.GetUserByID(ctx, other.UserID); err != nil || user == nil {
		t.Fatalf("unrelated member removed: %v %v", user, err)
	}
}

func TestSharedCloudAccountRemovalPreservesSubscription(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	u, err := s.CreateManagedCloudAccount(ctx, CreateManagedCloudAccountInput{Email: "shared-close@example.test", HashedPassword: "hash", TeamName: "Shared", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateManagedCloudAccount(ctx, CreateManagedCloudAccountInput{Email: "shared-successor@example.test", HashedPassword: "hash", TeamName: "Other", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, u.TenantID, v.UserID, TenantRoleOwner, u.UserID); err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite(ctx, u.UserID, "shared-billing.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCloudBillingAccount(ctx, CloudBillingAccount{TenantID: u.TenantID, PlanCode: CloudPlanPro, PlanName: "Pro", SubscriptionStatus: CloudSubscriptionStatusActive, StripeCustomerID: "cus_shared", StripeSubscriptionID: "sub_shared"}); err != nil {
		t.Fatal(err)
	}
	choices := api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: u.TenantID, UserID: v.UserID}}}
	plan, err := s.PlanAccountDeletion(ctx, u.UserID, choices, true)
	if err != nil || len(plan.Blockers) != 0 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if plan.Teams[0].CancellationRequired || plan.Teams[0].BillingFingerprint != "" {
		t.Fatal("shared subscription included in cancellation consequences")
	}
	hash, err := AccountDeletionConsequenceHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := s.SaveAccountDeletionIntent(ctx, AccountDeletionIntent{AttemptID: id, UserID: u.UserID, Choices: choices, Plan: plan, Cloud: true, ConsequenceHash: hash, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	m := NewTenantStoreManager(s, t.TempDir())
	t.Cleanup(func() { _ = m.Close() })
	if err := m.DeleteCurrentAccount(ctx, u.UserID, id, "jti", hash, func() bool { return true }, func(context.Context, api.AccountDeletionTeam, string) error {
		t.Error("shared subscription cancellation requested")
		return ErrAccountDeletionBlocked
	}); err != nil {
		t.Fatal(err)
	}
	billing, err := s.GetCloudBillingAccount(ctx, u.TenantID)
	if err != nil || billing.StripeSubscriptionID != "sub_shared" || billing.SubscriptionStatus != CloudSubscriptionStatusActive {
		t.Fatalf("shared billing changed: %v %v", billing, err)
	}
	if tenant, err := s.GetSiteTenantID(ctx, site.ID); err != nil || tenant != u.TenantID {
		t.Fatalf("site moved teams: %v %v", tenant, err)
	}
}

func TestSoloCloudClosureProviderFailureLeavesDataAndSafeRetryPurgesTenant(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	account, err := s.CreateManagedCloudAccount(ctx, CreateManagedCloudAccountInput{Email: "closing@example.test", HashedPassword: "hash", TeamName: "Solo", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateManagedCloudAccount(ctx, CreateManagedCloudAccountInput{Email: "unrelated@example.test", HashedPassword: "hash", TeamName: "Unrelated", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite(ctx, account.UserID, "closing.example.test")
	if err != nil {
		t.Fatal(err)
	}
	m := NewTenantStoreManager(s, t.TempDir())
	t.Cleanup(func() { _ = m.Close() })
	data, _, err := m.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.Exec(ctx, `INSERT INTO hits (site_id,session_id,page_id,timestamp,path) VALUES (?,?,?,NOW(),'/')`, site.ID, uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	choices := api.AccountDeletionChoices{CloseTeamIDs: []uuid.UUID{account.TenantID}}
	plan, err := s.PlanAccountDeletion(ctx, account.UserID, choices, true)
	if err != nil || len(plan.Blockers) != 0 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	hash, err := AccountDeletionConsequenceHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	attempt := uuid.NewString()
	if err := s.SaveAccountDeletionIntent(ctx, AccountDeletionIntent{AttemptID: attempt, UserID: account.UserID, ConsequenceHash: hash, Choices: choices, Plan: plan, Cloud: true, ExpiresAt: time.Now().Add(5 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	proofs := 0
	authorize := func() bool { proofs++; return true }
	err = m.DeleteCurrentAccount(ctx, account.UserID, attempt, "fresh-session", hash, authorize, func(context.Context, api.AccountDeletionTeam, string) error { return ErrAccountDeletionUnavailable })
	if !errors.Is(err, ErrAccountDeletionUnavailable) {
		t.Fatalf("provider error=%v", err)
	}
	if user, err := s.GetUserByID(ctx, account.UserID); err != nil || user == nil {
		t.Fatalf("provider failure removed user: %v %v", user, err)
	}
	var count int
	if err := data.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM hits WHERE site_id=?`, site.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("provider failure changed hits: %d %v", count, err)
	}
	if removing, err := s.AccountRemovalStarted(ctx, account.UserID); err != nil || removing {
		t.Fatalf("provider failure blocked access: %v %v", removing, err)
	}
	verified := func(ctx context.Context, team api.AccountDeletionTeam, _ string) error {
		return s.RecordClosedCloudAccount(ctx, ClosedCloudAccount{TeamID: team.ID, CustomerID: "cus_retained", SubscriptionID: "sub_retained", CancelAtPeriodEnd: true})
	}
	if err := m.DeleteCurrentAccount(ctx, account.UserID, attempt, "different-session", hash, authorize, verified); !errors.Is(err, ErrAccountDeletionProof) {
		t.Fatalf("changed session=%v", err)
	}
	if err := m.DeleteCurrentAccount(ctx, account.UserID, attempt, "fresh-session", hash, authorize, verified); err != nil {
		t.Fatal(err)
	}
	if proofs != 1 {
		t.Fatalf("consumed proof %d times", proofs)
	}
	if user, err := s.GetUserByID(ctx, account.UserID); err != nil || user != nil {
		t.Fatalf("removed user=%v err=%v", user, err)
	}
	if site, err := s.GetSiteByID(ctx, site.ID); err != nil || site != nil {
		t.Fatalf("closed site=%v err=%v", site, err)
	}
	if closed, err := s.IsRemovedCloudAccount(ctx, account.TenantID, "", ""); err != nil || !closed {
		t.Fatalf("retention receipt=%v err=%v", closed, err)
	}
	if user, err := s.GetUserByID(ctx, other.UserID); err != nil || user == nil {
		t.Fatalf("unrelated user=%v err=%v", user, err)
	}
	if err := m.DeleteCurrentAccount(ctx, account.UserID, attempt, "fresh-session", hash, authorize, verified); err != nil {
		t.Fatalf("completed retry=%v", err)
	}
	if proofs != 1 {
		t.Fatalf("completed retry consumed proof again: %d", proofs)
	}
}

func TestSoloCloudClosureCredentialChangeInvalidatesAuthorizedRetry(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	account, err := s.CreateManagedCloudAccount(ctx, CreateManagedCloudAccountInput{Email: "credential-retry@example.test", HashedPassword: "before", TeamName: "Solo", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	choices := api.AccountDeletionChoices{CloseTeamIDs: []uuid.UUID{account.TenantID}}
	plan, err := s.PlanAccountDeletion(ctx, account.UserID, choices, true)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := AccountDeletionConsequenceHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	attempt := uuid.NewString()
	if err := s.SaveAccountDeletionIntent(ctx, AccountDeletionIntent{AttemptID: attempt, UserID: account.UserID, ConsequenceHash: hash, Choices: choices, Plan: plan, Cloud: true, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	m := NewTenantStoreManager(s, t.TempDir())
	t.Cleanup(func() { _ = m.Close() })
	calls := 0
	stop := func(context.Context, api.AccountDeletionTeam, string) error {
		calls++
		return ErrAccountDeletionUnavailable
	}
	if err := m.DeleteCurrentAccount(ctx, account.UserID, attempt, "fresh-jti", hash, func() bool { return true }, stop); !errors.Is(err, ErrAccountDeletionUnavailable) {
		t.Fatalf("initial provider failure=%v", err)
	}
	if err := s.UpdatePasswordByID(ctx, account.UserID.String(), "after"); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteCurrentAccount(ctx, account.UserID, attempt, "fresh-jti", hash, nil, stop); !errors.Is(err, ErrAccountDeletionProof) {
		t.Fatalf("credential-changed retry=%v", err)
	}
	if calls != 1 {
		t.Fatal("invalidated authorization reached provider again")
	}
	if user, err := s.GetUserByID(ctx, account.UserID); err != nil || user == nil {
		t.Fatalf("invalid retry removed account: %v %v", user, err)
	}
}
