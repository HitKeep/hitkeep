//go:build billing

package database

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
)

func TestAccountDeletionCleansCreatorReferencesAndPersonalMessagesAcrossRestart(t *testing.T) {
	for _, mode := range []string{"committed_partial_cleanup", "existing_removing_intent"} {
		t.Run(mode, func(t *testing.T) {
			testAccountDeletionCreatorCleanupAcrossRestart(t, mode == "existing_removing_intent")
		})
	}
}

func testAccountDeletionCreatorCleanupAcrossRestart(t *testing.T, existingRemoving bool) {
	s := setupTenantStore(t)
	ctx := t.Context()
	departing, err := s.CreateUser(ctx, "cleanup-departing@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateUser(ctx, "cleanup-remaining@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, team, other, TenantRoleOwner, departing); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateInstanceRole(ctx, other, auth.InstanceOwner, departing); err != nil {
		t.Fatal(err)
	}
	sharedSite, err := s.CreateSite(ctx, departing, "cleanup-shared.example.test")
	if err != nil {
		t.Fatal(err)
	}
	otherSite, err := s.CreateSite(ctx, other, "cleanup-unrelated.example.test")
	if err != nil {
		t.Fatal(err)
	}
	type artifacts struct {
		qrID, importID      uuid.UUID
		qrToken, shareToken string
	}
	create := func(user, site uuid.UUID) artifacts {
		t.Helper()
		qr, token, err := s.CreateQRCode(ctx, site, user, api.QRCodeCreateRequest{Name: "Shared poster", DestinationURL: "https://destination.example.test/"})
		if err != nil {
			t.Fatal(err)
		}
		_, shareToken, err := s.CreateQRCodeShareLink(ctx, site, qr.ID, user)
		if err != nil {
			t.Fatal(err)
		}
		job, err := s.CreateSiteImportUpload(ctx, site, user, "plausible", []ImportFileCreate{{Filename: "visitors.csv", RelativePath: "synthetic/visitors.csv", SizeBytes: 7}})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkCloudLifecycleMessageSent(ctx, CloudLifecycleMessageUpdate{TenantID: team, UserID: user, Kind: CloudLifecycleMessageWelcome}); err != nil {
			t.Fatal(err)
		}
		return artifacts{qrID: qr.ID, importID: job.ID, qrToken: token, shareToken: shareToken}
	}
	personal, retained := create(departing, sharedSite.ID), create(other, otherSite.ID)
	if err := s.CreateQRCodeOpen(ctx, &api.QRCodeOpen{SiteID: sharedSite.ID, QRCodeID: personal.qrID, Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	choices := api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: team, UserID: other}}}
	plan, err := s.PlanAccountDeletion(ctx, departing, choices, false)
	if err != nil || len(plan.Blockers) != 0 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	hash, err := AccountDeletionConsequenceHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	attempt := uuid.NewString()
	if err := s.SaveAccountDeletionIntent(ctx, AccountDeletionIntent{AttemptID: attempt, UserID: departing, ConsequenceHash: hash, Choices: choices, Plan: plan, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	// This extra FK forces the real parent delete to fail after child cleanup commits.
	if err := s.Exec(ctx, `CREATE TABLE account_cleanup_failure_probe (user_id UUID REFERENCES users(id))`); err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(ctx, `INSERT INTO account_cleanup_failure_probe VALUES (?)`, departing); err != nil {
		t.Fatal(err)
	}
	dataPath := t.TempDir()
	manager := NewTenantStoreManager(s, dataPath)
	t.Cleanup(func() { _ = manager.Close() })
	proofs := 0
	if existingRemoving {
		// Model a previously approved journal stuck before the missing policies ran.
		if err := s.setAccountDeletionPhase(ctx, attempt, "fresh-jti", "removing"); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := manager.DeleteCurrentAccount(ctx, departing, attempt, "fresh-jti", hash, func() bool { proofs++; return true }, nil); err == nil {
			t.Fatal("expected injected parent FK failure")
		}
	}
	if removing, err := s.AccountRemovalStarted(ctx, departing); err != nil || !removing {
		t.Fatalf("removing=%v err=%v", removing, err)
	}
	if !existingRemoving {
		qr, err := s.GetQRCode(ctx, sharedSite.ID, personal.qrID)
		if err != nil || qr == nil || qr.CreatedBy != uuid.Nil {
			t.Fatalf("shared QR creator not cleared: %+v err=%v", qr, err)
		}
		if _, err := s.GetCloudLifecycleMessage(ctx, team, departing, CloudLifecycleMessageWelcome); !errors.Is(err, ErrCloudLifecycleMessageNotFound) {
			t.Fatalf("personal lifecycle message retained: %v", err)
		}
	}
	path := s.path
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := NewStore(path, WithThreads(1))
	if err := restarted.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	if err := restarted.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Exec(ctx, `DROP TABLE account_cleanup_failure_probe`); err != nil {
		t.Fatal(err)
	}
	resumed := NewTenantStoreManager(restarted, dataPath)
	t.Cleanup(func() { _ = resumed.Close() })
	if err := resumed.ResumeAccountRemovals(ctx); err != nil {
		t.Fatal(err)
	}
	if (!existingRemoving && proofs != 1) || (existingRemoving && proofs != 0) {
		t.Fatalf("fresh proof consumed %d times", proofs)
	}
	if user, err := restarted.GetUserByID(ctx, departing); err != nil || user != nil {
		t.Fatalf("departing user retained: %v %v", user, err)
	}
	if result, err := restarted.GetAccountDeletionResult(ctx, attempt, departing); err != nil || result.Status != "deleted" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, check := range []struct {
		site    uuid.UUID
		art     artifacts
		creator uuid.UUID
	}{{sharedSite.ID, personal, uuid.Nil}, {otherSite.ID, retained, other}} {
		qr, err := restarted.GetQRCodeByToken(ctx, check.art.qrToken)
		if err != nil || qr == nil || qr.ID != check.art.qrID || qr.CreatedBy != check.creator {
			t.Fatalf("QR changed: %+v err=%v", qr, err)
		}
		sharedQR, err := restarted.GetQRCodeByShareToken(ctx, check.art.shareToken)
		if err != nil || sharedQR == nil || sharedQR.ID != check.art.qrID {
			t.Fatalf("QR share lost: %+v err=%v", sharedQR, err)
		}
		shares, err := restarted.ListQRCodeShareLinks(ctx, check.site, check.art.qrID)
		if err != nil || len(shares) != 1 {
			t.Fatalf("share creator changed: %+v err=%v", shares, err)
		}
		var creator sql.NullString
		if err := restarted.DB().QueryRowContext(ctx, `SELECT CAST(created_by AS VARCHAR) FROM qr_code_share_links WHERE id=?`, shares[0].ID).Scan(&creator); err != nil || creator.Valid != (check.creator != uuid.Nil) || (creator.Valid && creator.String != check.creator.String()) {
			t.Fatalf("share creator changed: %+v err=%v", creator, err)
		}
		job, err := restarted.GetSiteImport(ctx, check.site, check.art.importID)
		if err != nil || job == nil || len(job.Files) != 1 || job.BytesTotal != 7 || (job.CreatedBy == nil) != (check.creator == uuid.Nil) || (check.creator != uuid.Nil && *job.CreatedBy != check.creator) {
			t.Fatalf("import changed: %+v err=%v", job, err)
		}
		if site, err := restarted.GetSiteByID(ctx, check.site); err != nil || site == nil || site.UserID != other {
			t.Fatalf("shared/other site changed: %+v err=%v", site, err)
		}
	}
	if count, err := restarted.CountQRCodeOpens(ctx, sharedSite.ID, personal.qrID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)); err != nil || count != 1 {
		t.Fatalf("shared QR analytics=%d err=%v", count, err)
	}
	if _, err := restarted.GetCloudLifecycleMessage(ctx, team, departing, CloudLifecycleMessageWelcome); !errors.Is(err, ErrCloudLifecycleMessageNotFound) {
		t.Fatalf("personal lifecycle message survived restart: %v", err)
	}
	if message, err := restarted.GetCloudLifecycleMessage(ctx, team, other, CloudLifecycleMessageWelcome); err != nil || message == nil {
		t.Fatalf("other lifecycle message removed: %+v err=%v", message, err)
	}
	if user, err := restarted.GetUserByID(ctx, other); err != nil || user == nil {
		t.Fatalf("other account removed: %+v err=%v", user, err)
	}
}
