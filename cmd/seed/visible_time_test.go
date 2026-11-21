package main

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/testutil/testdb"
)

func TestSeedVisibleTimeIncludesMeasuredAndUnmeasuredViews(t *testing.T) {
	ctx := t.Context()
	store := testdb.Shared(t)
	siteID := uuid.New()
	start := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	if err := store.Exec(ctx, `
		INSERT INTO hits (site_id, session_id, page_id, path, timestamp)
		SELECT ?, uuidv7(), uuidv7(), '/pricing', ?::TIMESTAMPTZ + n * INTERVAL '1 second'
		FROM range(128) AS steps(n)
	`, siteID, start); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := seedVisibleTime(ctx, store, siteID); err != nil {
			t.Fatal(err)
		}
	}
	pages, err := store.GetVisibleTimePages(ctx, api.AnalyticsParams{SiteID: siteID, Start: start, End: start.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].TrackedViews != 128 || pages[0].MeasuredViews < 1 || pages[0].MeasuredViews >= pages[0].TrackedViews {
		t.Fatalf("expected a mix of measured and unmeasured seeded views, got %+v", pages)
	}
}
