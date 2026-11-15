package database

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
	stduuid "uuid"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
)

func TestPersonalRemovalPreparationOmitsUnassignedSharedSitesAndOwners(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	owner, err := s.CreateUser(ctx, "private-owner@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateUser(ctx, "limited-member@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSite(ctx, owner, "unassigned-private.example.test"); err != nil {
		t.Fatal(err)
	}
	plan, err := s.PlanAccountDeletion(ctx, member, api.AccountDeletionChoices{}, false)
	if err != nil || len(plan.Blockers) != 0 || len(plan.Teams) != 1 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if len(plan.Teams[0].Sites) != 0 || len(plan.Teams[0].EligibleSuccessors) != 0 {
		t.Fatalf("unaffected private team metadata exposed: %+v", plan.Teams[0])
	}
}

func TestPersonalRemovalCanLeaveArchivedEmptyTeam(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	u, err := s.CreateUser(ctx, "archived-departing@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateUser(ctx, "archived-owner@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	defaultID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, defaultID, v, TenantRoleOwner, u); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateInstanceRole(ctx, v, auth.InstanceOwner, u); err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTenant(ctx, u, "Already archived", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveTenant(ctx, team.ID, u); err != nil {
		t.Fatal(err)
	}
	if err := s.RemovePersonalAccount(ctx, u, api.AccountDeletionChoices{}, false); err != nil {
		t.Fatal(err)
	}
	if user, err := s.GetUserByID(ctx, u); err != nil || user != nil {
		t.Fatalf("archived owner retained: %v %v", user, err)
	}
}

func TestPersonalRemovalRejectsOwnerFromDifferentTeam(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	u, err := s.CreateUser(ctx, "departing-cross@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateUser(ctx, "remaining-cross@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, team, v, TenantRoleOwner, u); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateInstanceRole(ctx, v, auth.InstanceOwner, u); err != nil {
		t.Fatal(err)
	}
	private, err := s.CreateTenant(ctx, u, "Private team", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSite(ctx, u, "same-tenant-only.example.test"); err != nil {
		t.Fatal(err)
	}
	choices := api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: private.ID, UserID: v}}}
	plan, err := s.PlanAccountDeletion(ctx, u, choices, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range plan.Blockers {
		if b.Code == "invalid_successor" && b.TeamID == private.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("cross-team owner was accepted: %+v", plan)
	}
	if err := s.RemovePersonalAccount(ctx, u, choices, false); !errors.Is(err, ErrAccountDeletionBlocked) {
		t.Fatalf("remove=%v", err)
	}
}

func TestPersonalRemovalResumesAfterCommittedCleanupParentFailure(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	u, err := s.CreateUser(ctx, "partial@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateUser(ctx, "partial-owner@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, team, v, TenantRoleOwner, u); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateInstanceRole(ctx, v, auth.InstanceOwner, u); err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite(ctx, u, "partial.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkSocialIdentity(ctx, LinkSocialIdentityInput{UserID: u, Provider: "apple", Subject: "retained-until-revoked"}); err != nil {
		t.Fatal(err)
	}
	choices := api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: team, UserID: v}}}
	plan, err := s.PlanAccountDeletion(ctx, u, choices, false)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := AccountDeletionConsequenceHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := s.SaveAccountDeletionIntent(ctx, AccountDeletionIntent{AttemptID: id, UserID: u, Choices: choices, Plan: plan, ConsequenceHash: hash, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(ctx, `CREATE TABLE deletion_failure_probe (user_id UUID REFERENCES users(id))`); err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(ctx, `INSERT INTO deletion_failure_probe VALUES (?)`, u); err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	m := NewTenantStoreManager(s, t.TempDir(), WithAccountDeletionProviderRevoker(func(ctx context.Context, userID uuid.UUID) []api.AccountDeletionProviderRevocation {
		providerCalls++
		identity, err := s.GetUserAppleIdentity(ctx, userID)
		if err != nil || identity == nil || identity.Subject != "retained-until-revoked" {
			t.Fatalf("provider called after identity cleanup: %v %v", identity, err)
		}
		started, err := s.AccountRemovalStarted(ctx, userID)
		if err != nil || !started {
			t.Fatalf("provider called before removal authorized: %v %v", started, err)
		}
		return []api.AccountDeletionProviderRevocation{{Provider: "apple", Status: "manual_revocation_required"}}
	}))
	t.Cleanup(func() { _ = m.Close() })
	proofs := 0
	err = m.DeleteCurrentAccount(ctx, u, id, "fresh-jti", hash, func() bool { proofs++; return true }, nil)
	if err == nil {
		t.Fatal("expected parent FK failure after committed child cleanup")
	}
	if started, err := s.AccountRemovalStarted(ctx, u); err != nil || !started {
		t.Fatalf("phase=%v err=%v", started, err)
	}
	if _, err := s.LinkSocialIdentity(ctx, LinkSocialIdentityInput{UserID: u, Provider: "google", Subject: "new-subject"}); err == nil {
		t.Fatal("identity linked after removal began")
	}
	if created, err := s.CreateTenant(ctx, u, "Created after removal began", ""); !errors.Is(err, ErrAccountDeletionBlocked) || created != nil {
		t.Fatalf("removing user created a team: team=%+v err=%v", created, err)
	}
	if err := s.Exec(ctx, `DROP TABLE deletion_failure_probe`); err != nil {
		t.Fatal(err)
	}
	if err := m.ResumeAccountRemovals(ctx); err != nil {
		t.Fatal(err)
	}
	var orphaned int
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants t WHERE NOT EXISTS (SELECT 1 FROM tenant_members m WHERE m.tenant_id=t.id) AND NOT EXISTS (SELECT 1 FROM tenant_archives a WHERE a.tenant_id=t.id)`).Scan(&orphaned); err != nil || orphaned != 0 {
		t.Fatalf("active teams without members after resume=%d err=%v", orphaned, err)
	}
	if providerCalls != 1 {
		t.Fatalf("provider revocation repeated after persisted result: %d", providerCalls)
	}
	result, err := s.GetAccountDeletionResult(ctx, id, u)
	if err != nil || len(result.ProviderRevocations) != 1 || result.ProviderRevocations[0].Status != "manual_revocation_required" {
		t.Fatalf("recovered provider result=%+v err=%v", result, err)
	}
	if proofs != 1 {
		t.Fatalf("proof consumed %d times", proofs)
	}
	if user, err := s.GetUserByID(ctx, u); err != nil || user != nil {
		t.Fatalf("user=%v err=%v", user, err)
	}
	if current, err := s.GetSiteByID(ctx, site.ID); err != nil || current == nil || current.UserID != v {
		t.Fatalf("site=%v err=%v", current, err)
	}
}

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

	_, personalToken, err := s.CreateAPIClient(ctx, departing, "Personal", "", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{site.ID: auth.SiteViewer}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, teamToken, err := s.CreateTeamAPIClient(ctx, teamID, "Team", "", map[uuid.UUID]auth.SiteRole{site.ID: auth.SiteViewer}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAPIClientAuth(ctx, personalToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAPIClientAuth(ctx, teamToken); err != nil {
		t.Fatal(err)
	}
	remember, err := s.CreateRememberMeToken(ctx, departing)
	if err != nil {
		t.Fatal(err)
	}
	reset, err := s.CreatePasswordResetToken(ctx, "departing@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUserPasskey(ctx, departing, "Key", "credential", "public-key", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.EnableUserTOTP(ctx, departing, "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkSocialIdentity(ctx, LinkSocialIdentityInput{UserID: departing, Provider: "google", Subject: "departing-linked-subject"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceUserRecoveryCodes(ctx, departing, []string{"hashed-code"}); err != nil {
		t.Fatal(err)
	}

	grants := []OAuthGrant{
		{ID: stduuid.NewV7(), UserID: stduuid.UUID(departing)},
		{ID: stduuid.NewV7(), UserID: stduuid.UUID(owner)},
	}
	signatures := make([]string, len(grants))
	op, err := s.BeginOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Rollback()
	for i := range grants {
		grant := &grants[i]
		grant.TeamID = stduuid.UUID(teamID)
		grant.ClientID, grant.ClientName, grant.Audience = "https://client.example.test", "Deletion fixture", "https://hitkeep.example.test/api"
		grant.Scopes, grant.SiteIDs = []string{"sites:read"}, []stduuid.UUID{stduuid.UUID(site.ID)}
		grant.CreatedAt, grant.ExpiresAt = time.Now(), time.Now().Add(time.Hour)
		if err := op.CreateGrant(ctx, *grant); err != nil {
			t.Fatal(err)
		}
		signatures[i] = fmt.Sprintf("%x", sha256.Sum256([]byte(grant.ID.String())))
		if err := op.CreateCredential(ctx, OAuthCredential{Kind: "refresh", Signature: signatures[i], GrantID: grant.ID, TeamID: grant.TeamID, Payload: []byte(`{"fixture":true}`), ExpiresAt: grant.ExpiresAt}); err != nil {
			t.Fatal(err)
		}
	}
	if err := op.Commit(); err != nil {
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
	if credential, err := s.GetAPIClientAuth(ctx, personalToken); err != nil || credential != nil {
		t.Fatalf("personal credential retained: %v %v", credential, err)
	}
	if credential, err := s.GetAPIClientAuth(ctx, teamToken); err != nil || credential == nil {
		t.Fatalf("team credential removed: %v %v", credential, err)
	}
	if id, err := s.ValidateRememberMeToken(ctx, remember); err == nil && id != uuid.Nil {
		t.Fatal("remember credential survived")
	}
	if _, err := s.ResolvePasswordResetEmail(ctx, reset); err == nil {
		t.Fatal("reset credential survived")
	}
	if keys, err := s.ListUserPasskeys(ctx, departing); err != nil || len(keys) != 0 {
		t.Fatalf("passkeys survived: %v %v", keys, err)
	}
	if identities, err := s.ListUserSocialIdentities(ctx, departing); err != nil || len(identities) != 0 {
		t.Fatalf("social identities survived: %v %v", identities, err)
	}

	if _, err := s.GetOAuthGrant(ctx, grants[0].ID); !errors.Is(err, ErrOAuthNotFound) {
		t.Fatalf("personal OAuth grant survived: %v", err)
	}
	if _, err := s.GetOAuthGrant(ctx, grants[1].ID); err != nil {
		t.Fatalf("unrelated OAuth grant removed: %v", err)
	}
	op, err = s.BeginOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Rollback()
	if _, err := op.GetCredential(ctx, "refresh", signatures[0]); !errors.Is(err, ErrOAuthNotFound) {
		t.Fatalf("personal OAuth refresh credential survived: %v", err)
	}
	if _, err := op.GetCredential(ctx, "refresh", signatures[1]); err != nil {
		t.Fatalf("unrelated OAuth refresh credential removed: %v", err)
	}
	if err := op.Rollback(); err != nil {
		t.Fatal(err)
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

func TestAccountDeletionSecurityFingerprintExcludesUsage(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	u, err := s.CreateUser(ctx, "credential-snapshot@example.test", "password")
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateUserPasskey(ctx, u, "Key", "fingerprint-credential", "public-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkSocialIdentity(ctx, LinkSocialIdentityInput{UserID: u, Provider: "google", Subject: "linked-subject"}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := s.AccountDeletionSecurityFingerprint(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePasskeySignCount(ctx, key, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkSocialIdentity(ctx, LinkSocialIdentityInput{UserID: u, Provider: "google", Subject: "linked-subject", MarkUsed: true}); err != nil {
		t.Fatal(err)
	}
	if current, err := s.AccountDeletionSecurityFingerprint(ctx, u); err != nil || current != fingerprint {
		t.Fatalf("ordinary factor use changed fingerprint: %v", err)
	}
	for _, change := range []struct {
		name  string
		apply func() error
	}{
		{"password", func() error { return s.UpdatePasswordByID(ctx, u.String(), "changed") }},
		{"totp", func() error { return s.EnableUserTOTP(ctx, u, "secret") }},
		{"recovery set", func() error { return s.ReplaceUserRecoveryCodes(ctx, u, []string{"hash"}) }},
		{"passkey removal", func() error { return s.DeleteUserPasskey(ctx, u, key) }},
		{"social removal", func() error { return s.DeleteSocialIdentityWithGuard(ctx, u, "google", true) }},
		{"email", func() error { return s.UpdateUserProfile(ctx, u, "changed-credential@example.test", "", "") }},
	} {
		t.Run(change.name, func(t *testing.T) {
			if err := change.apply(); err != nil {
				t.Fatal(err)
			}
			current, err := s.AccountDeletionSecurityFingerprint(ctx, u)
			if err != nil || current == fingerprint {
				t.Fatalf("credential change preserved fingerprint: %v", err)
			}
			fingerprint = current
		})
	}
}

func TestPersonalRemovalRejectsOwnershipAfterTenantSyncFailure(t *testing.T) {
	for _, departingRole := range []string{TenantRoleMember, TenantRoleOwner} {
		t.Run(departingRole, func(t *testing.T) {
			s := setupTenantStore(t)
			ctx := t.Context()
			owner, err := s.CreateUser(ctx, "sync-owner@example.test", "hash")
			if err != nil {
				t.Fatal(err)
			}
			departing, err := s.CreateUser(ctx, "sync-departing@example.test", "hash")
			if err != nil {
				t.Fatal(err)
			}
			target, err := s.CreateUser(ctx, "sync-target@example.test", "hash")
			if err != nil {
				t.Fatal(err)
			}
			team, err := s.CreateTenant(ctx, owner, "Surviving team", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.AddTeamMember(ctx, team.ID, departing, departingRole, owner); err != nil {
				t.Fatal(err)
			}
			if err := s.AddTeamMember(ctx, team.ID, target, TenantRoleMember, owner); err != nil {
				t.Fatal(err)
			}
			if err := s.SetActiveTenantID(ctx, departing, team.ID); err != nil {
				t.Fatal(err)
			}
			site, err := s.CreateSite(ctx, departing, "sync-retained.example.test")
			if err != nil {
				t.Fatal(err)
			}
			choices := api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: team.ID, UserID: owner}}}
			plan, err := s.PlanAccountDeletion(ctx, departing, choices, false)
			if err != nil || len(plan.Blockers) != 0 {
				t.Fatalf("plan=%+v err=%v", plan, err)
			}
			hash, err := AccountDeletionConsequenceHash(plan)
			if err != nil {
				t.Fatal(err)
			}
			attempt := uuid.NewString()
			if err := s.SaveAccountDeletionIntent(ctx, AccountDeletionIntent{AttemptID: attempt, UserID: departing, Choices: choices, Plan: plan, ConsequenceHash: hash, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
				t.Fatal(err)
			}
			dataPath := filepath.Join(t.TempDir(), "blocked-data-directory")
			if err := os.WriteFile(dataPath, []byte("block tenant directory creation"), 0o600); err != nil {
				t.Fatal(err)
			}
			m := NewTenantStoreManager(s, dataPath)
			t.Cleanup(func() { _ = m.Close() })
			if err := m.DeleteCurrentAccount(ctx, departing, attempt, "fresh-jti", hash, func() bool { return true }, nil); err == nil {
				t.Fatal("expected tenant sync failure after ownership reparenting")
			}
			if removing, err := s.AccountRemovalStarted(ctx, departing); err != nil || !removing {
				t.Fatalf("removing=%v err=%v", removing, err)
			}
			if current, err := s.GetSiteByID(ctx, site.ID); err != nil || current == nil || current.UserID != owner {
				t.Fatalf("site reparenting did not precede sync failure: %+v err=%v", current, err)
			}
			actorID, targetID := owner, departing
			if departingRole == TenantRoleOwner {
				actorID, targetID = departing, target
			}
			if err := s.TransferTeamOwnership(ctx, team.ID, actorID, targetID); !errors.Is(err, ErrAccountDeletionBlocked) {
				t.Fatalf("ownership changed with removing participant: %v", err)
			}
			if err := s.AddTeamMember(ctx, team.ID, departing, TenantRoleOwner, owner); !errors.Is(err, ErrAccountDeletionBlocked) {
				t.Fatalf("removing user promoted through membership write: %v", err)
			}
			if err := os.Remove(dataPath); err != nil {
				t.Fatal(err)
			}
			if err := m.ResumeAccountRemovals(ctx); err != nil {
				t.Fatal(err)
			}
			if user, err := s.GetUserByID(ctx, departing); err != nil || user != nil {
				t.Fatalf("departing user retained: %+v err=%v", user, err)
			}
			if role, err := s.GetTenantRole(ctx, team.ID, owner); err != nil || role != TenantRoleOwner {
				t.Fatalf("surviving owner lost authority: role=%q err=%v", role, err)
			}
			if owners, err := s.CountTeamOwners(ctx, team.ID); err != nil || owners != 1 {
				t.Fatalf("surviving team owners=%d err=%v", owners, err)
			}
			if current, err := s.GetSiteByID(ctx, site.ID); err != nil || current == nil || current.UserID != owner {
				t.Fatalf("surviving site changed: %+v err=%v", current, err)
			}
		})
	}
}
