package database

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
)

func TestPersonalRemovalPreservesSharedSitesAndRequiresSameTeamOwner(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	departing, err := s.CreateUser(ctx, "departing@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := s.CreateUser(ctx, "remaining@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, teamID, owner, TenantRoleOwner, departing); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateInstanceRole(ctx, owner, auth.InstanceOwner, departing); err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite(ctx, departing, "preserve.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(ctx, `INSERT INTO hits (site_id, session_id, page_id, timestamp, path) VALUES (?, ?, ?, NOW(), '/')`, site.ID, uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}

	choices := api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: teamID, UserID: owner}}}
	plan, err := s.PlanAccountDeletion(ctx, departing, choices, false)
	if err != nil || len(plan.Blockers) != 0 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if err := s.RemovePersonalAccount(ctx, departing, choices, false); err != nil {
		t.Fatal(err)
	}
	if user, err := s.GetUserByID(ctx, departing); err != nil || user != nil {
		t.Fatalf("user=%v err=%v", user, err)
	}
	got, err := s.GetSiteByID(ctx, site.ID)
	if err != nil || got == nil || got.ID != site.ID || got.UserID != owner {
		t.Fatalf("site=%+v err=%v", got, err)
	}
	var count int
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM hits WHERE site_id=?`, site.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("hits=%d err=%v", count, err)
	}
	if err := s.RemovePersonalAccount(ctx, departing, choices, false); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("retry: %v", err)
	}
}

func TestPersonalRemovalBlocksLastOperatorAndSharedSoleOwner(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	u, err := s.CreateUser(ctx, "operator@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.PlanAccountDeletion(ctx, u, api.AccountDeletionChoices{CloseTeamIDs: []uuid.UUID{team}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers) != 1 || plan.Blockers[0].Code != "last_operator" {
		t.Fatalf("plan=%+v", plan)
	}
	v, err := s.CreateUser(ctx, "member@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateInstanceRole(ctx, v, auth.InstanceOwner, u); err != nil {
		t.Fatal(err)
	}
	plan, err = s.PlanAccountDeletion(ctx, u, api.AccountDeletionChoices{CloseTeamIDs: []uuid.UUID{team}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers) != 1 || plan.Blockers[0].Code != "ownership_transfer_required" {
		t.Fatalf("plan=%+v", plan)
	}
	if err := s.RemovePersonalAccount(ctx, u, api.AccountDeletionChoices{}, false); !errors.Is(err, ErrAccountDeletionBlocked) {
		t.Fatalf("remove=%v", err)
	}
	if user, err := s.GetUserByID(ctx, u); user == nil || err != nil {
		t.Fatalf("blocked user=%v err=%v", user, err)
	}
}
