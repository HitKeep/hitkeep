//go:build billing

package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

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
