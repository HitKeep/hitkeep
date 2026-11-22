package database

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hitkeep/database/migrations"
	tenantmigrations "hitkeep/database/migrations/tenant"
)

// The base SQL fixtures are exact copies of the shared and tenant migrations
// under internal/database/migrations at e6b80be9231c9637b3d89ea32dfec69219de4169
// (also in v2.1.0), before 0c1da671 added hostname to the base CREATE statements.
type preHostnameMigrations struct {
	fs.ReadDirFS
	base    string
	fixture string
}

func (m preHostnameMigrations) Open(name string) (fs.File, error) {
	if name == m.base {
		return os.Open(m.fixture)
	}
	return m.ReadDirFS.Open(name)
}

func TestAnalyticsIndexMigrationPreservesPreHostnameHits(t *testing.T) {
	const hitColumns = `id, site_id, session_id, page_id, timestamp, path, hostname, referrer, user_agent,
		viewport_width, viewport_height, screen_width, screen_height, language, is_unique, country_code,
		utm_source, utm_medium, utm_campaign, utm_term, utm_content, region, city, provider, asn, asn_org, qr_code_id`
	for _, tc := range []struct {
		name, base, baseline, rebuild string
		schema                        fs.ReadDirFS
		open                          func(context.Context, string, ...StoreOption) (*Store, error)
	}{
		{"shared", "2025_05_11_000000_create_base_tables.sql", preAnalyticsIndexDropMigrationFile, dropIndexesMigrationFile, migrations.Fs, OpenMigratedStore},
		{"tenant", "0001_create_analytics_tables.sql", "0012_create_qr_campaign_analytics.sql", "0013_drop_analytics_art_indexes.sql", tenantmigrations.Fs, OpenMigratedTenantStore},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			path := filepath.Join(t.TempDir(), "pre-hostname.db")
			seed := NewStore(path, WithCheckpointInterval(0))
			if err := seed.Connect(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = seed.Close() })
			schema := preHostnameMigrations{tc.schema, tc.base, filepath.Join("testdata", "pre_hostname", tc.name, tc.base)}
			if err := applyMigrationBaseline(t, seed, schema, tc.baseline, ""); err != nil {
				t.Fatalf("apply pre-rebuild fixture: %v", err)
			}
			siteSQL := `INSERT INTO sites (id, domain) VALUES (uuid(), 'legacy.example')`
			if tc.name == "shared" {
				siteSQL = `INSERT INTO users (id, email, password, created_at) VALUES (uuid(), 'legacy@example.com', 'hash', now());
					INSERT INTO sites (id, user_id, domain, created_at) SELECT uuid(), id, 'legacy.example', now() FROM users`
			}
			if err := seed.Exec(ctx, siteSQL); err != nil {
				t.Fatalf("seed legacy site: %v", err)
			}
			if err := seed.Exec(ctx, `
				INSERT INTO hits (`+hitColumns+`)
				SELECT uuid(), id, uuid(), uuid(), TIMESTAMPTZ '2026-06-15 12:34:56+00', '/complete',
					'landing.legacy.example', 'https://referrer.example/path', 'sentinel agent',
					701, 702, 703, 704, 'en-US', true, 'US', 'source', 'medium', 'campaign',
					'term', 'content', 'region', 'city', 'provider', 64512, 'asn org', uuid()
				FROM sites WHERE domain = 'legacy.example';
				INSERT INTO hits (id, site_id, session_id, page_id, timestamp, path, is_unique)
				SELECT uuid(), id, uuid(), uuid(), TIMESTAMPTZ '2026-06-16 13:35:57+00', '/sparse', false
				FROM sites WHERE domain = 'legacy.example'`); err != nil {
				t.Fatalf("seed historical hits: %v", err)
			}
			snapshot := func(store *Store) string {
				var count int
				var data string
				if err := store.DB().QueryRowContext(ctx, `
					SELECT count(*), to_json(list(h ORDER BY h.path))::VARCHAR FROM (SELECT `+hitColumns+` FROM hits) h
				`).Scan(&count, &data); err != nil || count != 2 {
					t.Fatalf("read hit snapshot: count=%d, want 2, err=%v", count, err)
				}
				return data
			}
			before := snapshot(seed)
			if err := seed.Close(); err != nil {
				t.Fatal(err)
			}

			store, err := tc.open(ctx, path, WithCheckpointInterval(0))
			if err != nil {
				t.Fatalf("upgrade pre-hostname database: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if after := snapshot(store); after != before {
				t.Fatalf("historical hit values changed\nbefore: %s\nafter:  %s", before, after)
			}
			appliedAt := func(store *Store) time.Time {
				var count int
				var timestamp time.Time
				if err := store.DB().QueryRowContext(ctx,
					"SELECT count(*), min(applied_at) FROM migrations WHERE migration = ?", tc.rebuild,
				).Scan(&count, &timestamp); err != nil || count != 1 {
					t.Fatalf("read rebuild journal: count=%d, want 1, err=%v", count, err)
				}
				return timestamp
			}
			applied := appliedAt(store)
			if err := store.Exec(ctx, "UPDATE hits SET paid_click = true WHERE path = '/complete'"); err != nil {
				t.Fatalf("write current-schema hit: %v", err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}

			reopened, err := tc.open(ctx, path, WithCheckpointInterval(0))
			if err != nil {
				t.Fatalf("reopen already-migrated database: %v", err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			if after := appliedAt(reopened); !after.Equal(applied) {
				t.Fatalf("completed rebuild was replayed: %v became %v", applied, after)
			}
			var paid bool
			if err := reopened.DB().QueryRowContext(ctx, "SELECT paid_click FROM hits WHERE path = '/complete'").Scan(&paid); err != nil || !paid {
				t.Fatalf("read paid flag after reopening: paid=%v err=%v", paid, err)
			}
			var generatedVersion string
			var defaultPaid *bool
			if err := reopened.DB().QueryRowContext(ctx, `
				INSERT INTO hits (site_id, session_id, page_id, timestamp, path)
				SELECT id, uuid(), uuid(), now(), '/defaults' FROM sites WHERE domain = 'legacy.example'
				RETURNING substring(id::VARCHAR, 15, 1), paid_click
			`).Scan(&generatedVersion, &defaultPaid); err != nil || generatedVersion != "7" || defaultPaid != nil {
				t.Fatalf("read rebuilt hit defaults: UUID version=%q, want 7, paid=%v, want NULL, err=%v", generatedVersion, defaultPaid, err)
			}
		})
	}
}
