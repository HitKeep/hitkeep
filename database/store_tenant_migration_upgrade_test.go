package database

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/database/migrations"
	tenantmigrations "hitkeep/database/migrations/tenant"
)

func TestTenantMigrationUpgradeWithExistingSiteReferences(t *testing.T) {
	ctx := t.Context()

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := applyMigrationsThrough(t, store, "2026_04_01_000000_create_report_subscriptions.sql"); err != nil {
		t.Fatalf("apply baseline migrations: %v", err)
	}

	now := time.Now().UTC()
	userID := uuid.New()
	siteID := uuid.New()
	hitID := uuid.New()
	sessionID := uuid.New()
	pageID := uuid.New()

	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO users (id, email, password, created_at) VALUES (?, ?, ?, ?)",
		userID, "upgrade@tenant.test", "hash", now,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, ?, ?)",
		siteID, userID, "upgrade-tenant.test", now,
	); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO hits (
			id, site_id, session_id, page_id, timestamp, path
		) VALUES (?, ?, ?, ?, ?, ?)
	`, hitID, siteID, sessionID, pageID, now, "/"); err != nil {
		t.Fatalf("insert hit: %v", err)
	}

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store with tenant migrations: %v", err)
	}

	var paidClick *bool
	if err := store.DB().QueryRowContext(ctx, "SELECT paid_click FROM hits WHERE id = ?", hitID).Scan(&paidClick); err != nil {
		t.Fatalf("query legacy hit after migration: %v", err)
	}
	if paidClick != nil {
		t.Fatal("expected legacy hit to remain unmeasured")
	}

	matched, err := store.applyPageVisibleTime(ctx, []api.PageVisibleTimeSample{{SiteID: siteID, PageID: pageID, VisibleMS: 1200}})
	if err != nil || len(matched) != 1 || !matched[0] {
		t.Fatalf("write visible time for an upgraded hit: matched=%v err=%v", matched, err)
	}

	var tenantMappingCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_tenants WHERE site_id = ?",
		siteID,
	).Scan(&tenantMappingCount); err != nil {
		t.Fatalf("query site_tenants: %v", err)
	}
	if tenantMappingCount != 1 {
		t.Fatalf("expected 1 site_tenants row for site, got %d", tenantMappingCount)
	}

	var tenantMemberCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tenant_members WHERE user_id = ?",
		userID,
	).Scan(&tenantMemberCount); err != nil {
		t.Fatalf("query tenant_members: %v", err)
	}
	if tenantMemberCount != 1 {
		t.Fatalf("expected 1 tenant_members row for user, got %d", tenantMemberCount)
	}
}

func TestVisibleTimeMigrationAfterPaidClick(t *testing.T) {
	for _, tc := range []struct {
		name    string
		schema  fs.FS
		paid    string
		visible string
		migrate func(*Store, context.Context) error
	}{
		{"shared", migrations.Fs, "2026_11_21_030000_add_paid_click.sql", "2026_11_21_020000_create_page_visible_time.sql", (*Store).Migrate},
		{"tenant", tenantmigrations.Fs, "0021_add_paid_click.sql", "0020_create_page_visible_time.sql", (*Store).MigrateTenant},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			store := NewStore(":memory:")
			if err := store.Connect(); err != nil {
				t.Fatalf("connect store: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := applyMigrationBaseline(t, store, tc.schema, tc.paid, tc.visible); err != nil {
				t.Fatalf("apply paid-only migrations: %v", err)
			}

			siteID := uuid.New()
			if tc.name == "shared" {
				userID, err := store.CreateUser(ctx, "paid-upgrade@example.com", "hash")
				if err != nil {
					t.Fatal(err)
				}
				site, err := store.CreateSite(ctx, userID, "paid-upgrade.example.com")
				if err != nil {
					t.Fatal(err)
				}
				siteID = site.ID
			} else if err := store.UpsertSiteMirror(ctx, &api.Site{ID: siteID, Domain: "paid-upgrade.example.com"}); err != nil {
				t.Fatal(err)
			}
			pageID := uuid.New()
			if err := store.Exec(ctx, `INSERT INTO hits (site_id, session_id, page_id, timestamp, path, paid_click) VALUES (?, ?, ?, now(), '/', true)`, siteID, uuid.New(), pageID); err != nil {
				t.Fatalf("insert measured paid click: %v", err)
			}

			if err := tc.migrate(store, ctx); err != nil {
				t.Fatalf("upgrade paid-only database: %v", err)
			}
			matched, err := store.applyPageVisibleTime(ctx, []api.PageVisibleTimeSample{{SiteID: siteID, PageID: pageID, VisibleMS: 1200}})
			if err != nil || len(matched) != 1 || !matched[0] {
				t.Fatalf("write visible time for paid hit: matched=%v err=%v", matched, err)
			}
			var paid bool
			var visibleMS int
			if err := store.DB().QueryRowContext(ctx, `
				SELECT h.paid_click, v.visible_ms FROM hits h JOIN page_visible_time v USING (site_id, page_id)
				WHERE h.site_id = ? AND h.page_id = ?
			`, siteID, pageID).Scan(&paid, &visibleMS); err != nil || !paid || visibleMS != 1200 {
				t.Fatalf("paid=%v visible_ms=%d, want true and 1200: %v", paid, visibleMS, err)
			}
		})
	}
}

func TestCentralAuditMigrationBackfillsTeamAuditView(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := applyMigrationsThrough(t, store, "2026_05_22_000000_normalize_imported_property_event_names.sql"); err != nil {
		t.Fatalf("apply baseline migrations: %v", err)
	}

	actorID := uuid.New()
	targetID := uuid.New()
	for _, legacyUser := range []struct {
		id    uuid.UUID
		email string
	}{
		{id: actorID, email: "legacy-audit-actor@example.com"},
		{id: targetID, email: "legacy-audit-target@example.com"},
	} {
		if _, err := store.DB().ExecContext(ctx,
			"INSERT INTO users (id, email, password, created_at) VALUES (?, ?, ?, ?)",
			legacyUser.id, legacyUser.email, "hash", time.Now().UTC(),
		); err != nil {
			t.Fatalf("insert legacy user: %v", err)
		}
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	legacyAuditID := uuid.New()
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO team_audit_log (id, tenant_id, actor_id, target_user_id, action, details)
		VALUES (?, ?, ?, ?, ?, ?)
	`, legacyAuditID, tenantID, actorID, targetID, "member.added", "legacy row"); err != nil {
		t.Fatalf("insert legacy team audit row: %v", err)
	}

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate central audit: %v", err)
	}

	var centralCount int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM instance_audit_log
		WHERE id = ? AND team_id = ? AND target_user_id = ? AND action = ?
	`, legacyAuditID, tenantID, targetID, "member.added").Scan(&centralCount); err != nil {
		t.Fatalf("count central backfill rows: %v", err)
	}
	if centralCount != 1 {
		t.Fatalf("expected one central backfill row, got %d", centralCount)
	}

	entries, total, err := store.ListTeamAuditEntries(ctx, tenantID, "member.added", 10, 0)
	if err != nil {
		t.Fatalf("list team audit view entries: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one projected team audit row, total=%d len=%d", total, len(entries))
	}
	if entries[0].ID != legacyAuditID {
		t.Fatalf("expected projected legacy audit id %s, got %s", legacyAuditID, entries[0].ID)
	}
}

func applyMigrationsThrough(t *testing.T, store *Store, maxFileName string) error {
	t.Helper()
	return applyMigrationBaseline(t, store, migrations.Fs, maxFileName, "")
}

func applyMigrationBaseline(t *testing.T, store *Store, schema fs.FS, maxFileName, skip string) error {
	t.Helper()

	ctx := t.Context()
	files, err := fs.Glob(schema, "*.sql")
	if err != nil {
		return err
	}
	for _, name := range files {
		if name > maxFileName {
			break
		}
		if name == skip {
			continue
		}
		contents, err := fs.ReadFile(schema, name)
		if err != nil {
			return err
		}
		if _, err := store.DB().ExecContext(ctx, string(contents)); err != nil {
			return err
		}
		if name == "0000_00_00_000000_create_migrations_table.sql" {
			continue
		}
		if _, err := store.DB().ExecContext(ctx, "INSERT INTO migrations (migration, applied_at) VALUES (?, ?)", name, time.Now().UTC()); err != nil {
			return err
		}
	}

	return nil
}
