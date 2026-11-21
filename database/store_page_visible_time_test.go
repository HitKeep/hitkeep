package database

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

func TestPageVisibleTimeRequiresScopedPageviewAndStoresMaximum(t *testing.T) {
	ctx := t.Context()
	store, _, site := setupAppenderStore(t)
	pageID := uuid.New()
	sample := api.PageVisibleTimeSample{SiteID: site.ID, PageID: pageID}
	write := func(want []bool, samples ...api.PageVisibleTimeSample) {
		t.Helper()
		matched, err := store.applyPageVisibleTime(ctx, samples)
		if err != nil || !slices.Equal(matched, want) {
			t.Fatalf("apply page visible time: matched=%v want=%v err=%v", matched, want, err)
		}
	}
	assertCount := func(want int) {
		t.Helper()
		var count int
		if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM page_visible_time").Scan(&count); err != nil || count != want {
			t.Fatalf("visible time count=%d want=%d err=%v", count, want, err)
		}
	}
	assertMilliseconds := func(want int) {
		t.Helper()
		var ms int
		if err := store.DB().QueryRowContext(ctx, "SELECT visible_ms FROM page_visible_time WHERE site_id = ? AND page_id = ?", site.ID, pageID).Scan(&ms); err != nil || ms != want {
			t.Fatalf("visible ms=%d want=%d err=%v", ms, want, err)
		}
	}

	write([]bool{false}, sample)
	assertCount(0)
	hit := api.Hit{SiteID: site.ID, PageID: pageID, SessionID: uuid.New(), Path: "/", Timestamp: time.Now()}
	if err := store.CreateHit(ctx, &hit); err != nil {
		t.Fatalf("create hit: %v", err)
	}
	otherSite, err := store.CreateSite(ctx, site.UserID, "other-visible.example")
	if err != nil {
		t.Fatalf("create another site: %v", err)
	}
	otherSiteSample := api.PageVisibleTimeSample{SiteID: otherSite.ID, PageID: pageID, VisibleMS: 1000}
	write([]bool{true, false}, sample, otherSiteSample)
	assertMilliseconds(0)

	later := sample
	later.VisibleMS = api.MaxPageVisibleTimeMS
	write([]bool{true, true, true}, later, sample, later)
	write([]bool{true, true}, sample, later)
	assertCount(1)
	assertMilliseconds(api.MaxPageVisibleTimeMS)

	if _, err := store.ResetSiteStats(ctx, site.ID); err != nil {
		t.Fatalf("reset stats: %v", err)
	}
	write([]bool{false}, later)
	assertCount(0)
}

func BenchmarkApplyPageVisibleTime(b *testing.B) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	ctx := b.Context()
	if err := store.MigrateTenant(ctx); err != nil {
		b.Fatal(err)
	}
	siteID := uuid.New()
	if err := store.UpsertSiteMirror(ctx, &api.Site{ID: siteID, Domain: "benchmark.test"}); err != nil {
		b.Fatal(err)
	}
	if err := store.Exec(ctx, `
		INSERT INTO hits (site_id, page_id, session_id, timestamp, path)
		SELECT ?, uuid(), uuid(), now(), '/' FROM range(100000)
	`, siteID); err != nil {
		b.Fatal(err)
	}
	rows, err := store.DB().QueryContext(ctx, "SELECT page_id FROM hits LIMIT 64")
	if err != nil {
		b.Fatal(err)
	}
	defer rows.Close()
	var samples []api.PageVisibleTimeSample
	for rows.Next() {
		sample := api.PageVisibleTimeSample{SiteID: siteID}
		if err := rows.Scan(&sample.PageID); err != nil {
			b.Fatal(err)
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		b.Fatal(err)
	}
	if len(samples) != 64 {
		b.Fatalf("expected 64 benchmark samples, got %d", len(samples))
	}

	b.Run("updates_64", func(b *testing.B) {
		for b.Loop() {
			for i := range samples {
				samples[i].VisibleMS++
			}
			if _, err := store.applyPageVisibleTime(ctx, samples); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("duplicates_64", func(b *testing.B) {
		for b.Loop() {
			if _, err := store.applyPageVisibleTime(ctx, samples); err != nil {
				b.Fatal(err)
			}
		}
	})
}
