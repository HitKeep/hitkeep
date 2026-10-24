package database

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestOAuthOperationRollbackAndConsumedCredential(t *testing.T) {
	store, oldUser, oldSite := setupAPIClientStore(t)
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	oldTeam, err := store.GetSiteTenantID(ctx, oldSite)
	if err != nil {
		t.Fatal(err)
	}
	var applied int
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM migrations WHERE migration=?", "2026_10_20_000000_create_oauth.sql").Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("canonical OAuth migration not applied: count=%d err=%v", applied, err)
	}
	grant := OAuthGrant{ID: uuid.NewV7(), UserID: uuid.UUID(oldUser), TeamID: uuid.UUID(oldTeam), ClientID: "https://client.example/metadata", ClientName: "Example", Audience: "https://hitkeep.example/api", Scopes: []string{"sites:read"}, SiteIDs: []uuid.UUID{uuid.UUID(oldSite)}, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour)}
	op, err := store.BeginOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.CreateGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if err := op.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOAuthGrant(ctx, grant.ID); !errors.Is(err, ErrOAuthNotFound) {
		t.Fatalf("rolled back grant visible: %v", err)
	}
	op, err = store.BeginOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Rollback()
	if err := op.CreateGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	credential := OAuthCredential{Kind: "refresh", Signature: fmt.Sprintf("%x", sha256.Sum256([]byte("credential-canary"))), GrantID: grant.ID, TeamID: grant.TeamID, Payload: []byte(`{"safe":true}`), ExpiresAt: grant.ExpiresAt}
	if err := op.CreateCredential(ctx, credential); err != nil {
		t.Fatal(err)
	}
	if err := op.ConsumeCredential(ctx, "refresh", credential.Signature); err != nil {
		t.Fatal(err)
	}
	if err := op.ConsumeCredential(ctx, "refresh", credential.Signature); !errors.Is(err, ErrOAuthConsumed) {
		t.Fatalf("credential reused: %v", err)
	}
	if err := op.Commit(); err != nil {
		t.Fatal(err)
	}
	op, err = store.BeginOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Rollback()
	loaded, err := op.GetCredential(ctx, "refresh", credential.Signature)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Consumed {
		t.Fatal("consumption lost across commit")
	}
	if err := op.RevokeGrant(ctx, grant.ID); err != nil {
		t.Fatal(err)
	}
	if err := op.Commit(); err != nil {
		t.Fatal(err)
	}
	loadedGrant, err := store.GetOAuthGrant(ctx, grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedGrant.RevokedAt == nil {
		t.Fatal("family revocation was not durable")
	}
}

func TestOAuthGrantOwnerRevokeAndCurrentTenancy(t *testing.T) {
	store, oldUser, oldSite := setupAPIClientStore(t)
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	oldTeam, err := store.GetSiteTenantID(ctx, oldSite)
	if err != nil {
		t.Fatal(err)
	}
	grant := OAuthGrant{ID: uuid.NewV7(), UserID: uuid.UUID(oldUser), TeamID: uuid.UUID(oldTeam), ClientID: "https://client.example/metadata", ClientName: "Example", Audience: "https://hitkeep.example/api", Scopes: []string{"sites:read"}, SiteIDs: []uuid.UUID{uuid.UUID(oldSite)}, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	op, err := store.BeginOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Rollback()
	if err := op.CreateGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if err := op.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeOAuthGrant(ctx, uuid.NewV7(), grant.ID); !errors.Is(err, ErrOAuthNotFound) {
		t.Fatalf("foreign owner revoked grant: %v", err)
	}
	if err := store.RevokeOAuthGrant(ctx, grant.UserID, grant.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetOAuthGrant(ctx, grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RevokedAt == nil {
		t.Fatal("owner revoke did not persist")
	}
}

func TestOAuthGrantFollowsUserShadowProfileAndDeletion(t *testing.T) {
	store, _, oldSite := setupAPIClientStore(t)
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	oldTeam, err := store.GetSiteTenantID(ctx, oldSite)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(ctx, "oauth-member@example.test", "unused-hash")
	if err != nil {
		t.Fatal(err)
	}
	grant := OAuthGrant{ID: uuid.NewV7(), UserID: uuid.UUID(user), TeamID: uuid.UUID(oldTeam), ClientID: "https://client.example/metadata", ClientName: "Example", Audience: "https://hitkeep.example/api", Scopes: []string{"sites:read"}, SiteIDs: []uuid.UUID{uuid.UUID(oldSite)}, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	op, err := store.BeginOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Rollback()
	if err := op.CreateGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if err := op.CreateCredential(ctx, OAuthCredential{Kind: "access", Signature: fmt.Sprintf("%x", sha256.Sum256([]byte("profile-canary"))), GrantID: grant.ID, TeamID: grant.TeamID, Payload: []byte(`{"safe":true}`), ExpiresAt: grant.ExpiresAt}); err != nil {
		t.Fatal(err)
	}
	if err := op.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateUserProfile(ctx, user, "oauth-member-updated@example.test", "Changed", "Name"); err != nil {
		_, tail, _ := strings.Cut(err.Error(), "user_id: ")
		shadowID := ""
		if fields := strings.Fields(tail); len(fields) > 0 {
			shadowID = strings.Trim(fields[0], "\"")
		}
		t.Logf("exact failed shadow user=%s original=%s", shadowID, user)
		rows, queryErr := store.DB().QueryContext(ctx, "SELECT table_name, unnest(constraint_column_names) FROM duckdb_constraints() WHERE constraint_type='FOREIGN KEY' AND referenced_table='users'")
		if queryErr == nil {
			var refs [][2]string
			for rows.Next() {
				var table, column string
				if rows.Scan(&table, &column) == nil {
					refs = append(refs, [2]string{table, column})
				}
			}
			rows.Close()
			for _, ref := range refs {
				var count int
				query := "SELECT count(*) FROM \"" + ref[0] + "\" WHERE \"" + ref[1] + "\" = ?"
				queryErr := store.DB().QueryRowContext(ctx, query, shadowID).Scan(&count)
				t.Logf("shadow reference %s.%s count=%d err=%v", ref[0], ref[1], count, queryErr)
			}
		} else {
			t.Logf("constraint diagnostic: %v", queryErr)
		}
		t.Fatalf("OAuth grant regression in shadow profile update: %v", err)
	}
	got, err := store.GetOAuthGrant(ctx, grant.ID)
	if err != nil || got.UserID != grant.UserID {
		t.Fatalf("shadow profile changed grant owner: %v %+v", err, got)
	}
	if err := store.DeleteUser(ctx, user); err != nil {
		t.Fatalf("OAuth grant breaks existing user deletion: %v", err)
	}
	if _, err := store.GetOAuthGrant(ctx, grant.ID); !errors.Is(err, ErrOAuthNotFound) {
		t.Fatalf("deleted user's grant survived: %v", err)
	}
	for _, table := range []string{"oauth_credentials", "oauth_grant_sites"} {
		var count int
		if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE oauth_grant_id=?", grant.ID.String()).Scan(&count); err != nil || count != 0 {
			t.Fatalf("orphan OAuth child %s count=%d err=%v", table, count, err)
		}
	}
}

func TestOAuthGrantSiteDeletionIsSchemaDerived(t *testing.T) {
	store, oldUser, oldSite := setupAPIClientStore(t)
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	oldTeam, err := store.GetSiteTenantID(ctx, oldSite)
	if err != nil {
		t.Fatal(err)
	}
	grant := OAuthGrant{ID: uuid.NewV7(), UserID: uuid.UUID(oldUser), TeamID: uuid.UUID(oldTeam), ClientID: "https://client.example/metadata", ClientName: "Example", Audience: "https://hitkeep.example/api", Scopes: []string{"sites:read"}, SiteIDs: []uuid.UUID{uuid.UUID(oldSite)}, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	op, err := store.BeginOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Rollback()
	if err := op.CreateGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if err := op.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSite(ctx, oldSite); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetOAuthGrant(ctx, grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active || len(got.SiteIDs) != 0 {
		t.Fatalf("deleted site still authorized: %+v", got)
	}
}

func TestOAuthCrashHelper(t *testing.T) {
	path := os.Getenv("HITKEEP_OAUTH_CRASH_DB")
	if path == "" {
		t.Skip("subprocess helper")
	}
	store := NewStore(path)
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	op, err := store.BeginOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	old, err := op.GetCredential(ctx, "refresh", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := op.ConsumeCredential(ctx, "refresh", old.Signature); err != nil {
		t.Fatal(err)
	}
	if err := op.CreateCredential(ctx, OAuthCredential{Kind: "refresh", Signature: strings.Repeat("b", 64), GrantID: old.GrantID, TeamID: old.TeamID, Payload: []byte(`{"safe":true}`), ExpiresAt: old.ExpiresAt}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("HITKEEP_OAUTH_CRASH_COMMIT") == "yes" {
		if err := op.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	// Exit without rollback, Store.Close or a checkpoint to exercise DuckDB recovery.
	os.Exit(0)
}

func TestOAuthCrashRecoveryKeepsExchangeAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oauth-crash.db")
	open := func() *Store {
		s := NewStore(path)
		if err := s.Connect(); err != nil {
			t.Fatal(err)
		}
		return s
	}
	store := open()
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(t.Context(), "oauth-crash@example.test", "unused-hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(t.Context(), user, "oauth-crash.example")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.GetSiteTenantID(t.Context(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	grant := OAuthGrant{ID: uuid.NewV7(), UserID: uuid.UUID(user), TeamID: uuid.UUID(team), ClientID: "test-client", ClientName: "Test", Audience: "https://hitkeep.example/api", Scopes: []string{"sites:read"}, SiteIDs: []uuid.UUID{uuid.UUID(site.ID)}, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	op, err := store.BeginOAuth(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := op.CreateGrant(t.Context(), grant); err != nil {
		t.Fatal(err)
	}
	if err := op.CreateCredential(t.Context(), OAuthCredential{Kind: "refresh", Signature: strings.Repeat("a", 64), GrantID: grant.ID, TeamID: grant.TeamID, Payload: []byte(`{"safe":true}`), ExpiresAt: grant.ExpiresAt}); err != nil {
		t.Fatal(err)
	}
	if err := op.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, commit := range []bool{false, true} {
		mode := "no"
		if commit {
			mode = "yes"
		}
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestOAuthCrashHelper$")
		cmd.Env = append(os.Environ(), "HITKEEP_OAUTH_CRASH_DB="+path, "HITKEEP_OAUTH_CRASH_COMMIT="+mode)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("crash subprocess: %v %s", err, output)
		}
		store = open()
		op, err = store.BeginOAuth(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		old, err := op.GetCredential(t.Context(), "refresh", strings.Repeat("a", 64))
		if err != nil {
			t.Fatal(err)
		}
		_, newErr := op.GetCredential(t.Context(), "refresh", strings.Repeat("b", 64))
		if old.Consumed != commit || (commit && newErr != nil) || (!commit && !errors.Is(newErr, ErrOAuthNotFound)) {
			t.Fatalf("partial exchange after crash commit=%v consumed=%v replacement=%v", commit, old.Consumed, newErr)
		}
		if err := op.Rollback(); err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOAuthProfileBaselineWithoutGrant(t *testing.T) {
	store, _, _ := setupAPIClientStore(t)
	t.Cleanup(func() { _ = store.Close() })
	user, err := store.CreateUser(t.Context(), "oauth-profile-baseline@example.test", "unused-hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateUserProfile(t.Context(), user, "oauth-profile-baseline-updated@example.test", "Changed", "Name"); err != nil {
		t.Fatalf("profile baseline without OAuth rows: %v", err)
	}
}
