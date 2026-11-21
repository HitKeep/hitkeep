package database

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/importables"
)

func TestImportedStatsPreserveNativeBounceCount(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := t.Context()
	site, err := store.CreateSite(ctx, userID, "bounce.example.test")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, duration := range []time.Duration{0, 20 * time.Second, 40 * time.Second} {
		session := uuid.New()
		if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: session, PageID: uuid.New(), Timestamp: at, Path: "/first"}); err != nil {
			t.Fatal(err)
		}
		if duration > 0 {
			if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: session, PageID: uuid.New(), Timestamp: at.Add(duration), Path: "/second"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	params := api.AnalyticsParams{SiteID: site.ID, Start: dateOnly(at.AddDate(0, 0, -1)), End: at.AddDate(0, 0, 1)}
	for _, imported := range []bool{false, true} {
		if imported {
			sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: at.AddDate(0, 0, -1), Visits: 1, Visitors: 1, Pageviews: 1, Bounces: 1, VisitDuration: 120, SourceFile: "visitors.csv"}); err != nil {
				t.Fatal(err)
			}
			if err := sink.Flush(ctx); err != nil {
				t.Fatal(err)
			}
		}
		wantBounce, wantDuration := 100.0/3, 20.0
		if imported {
			wantBounce, wantDuration = 50, 45
		}
		stats, err := store.GetSiteStats(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(stats.BounceRate-wantBounce) > 1e-9 || stats.AvgSessionDuration != wantDuration {
			t.Errorf("imported=%t: bounce=%v duration=%v, want %v and %v", imported, stats.BounceRate, stats.AvgSessionDuration, wantBounce, wantDuration)
		}
		overview, err := store.GetSiteOverviewStats(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(overview.BounceRate-wantBounce) > 1e-9 {
			t.Errorf("imported=%t overview bounce=%v, want %v", imported, overview.BounceRate, wantBounce)
		}
	}
}
