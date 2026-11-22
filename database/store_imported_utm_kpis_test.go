package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/importables"
)

func TestSiteStatsImportedUTMPageviewKPIs(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := t.Context()
	site, err := store.CreateSite(ctx, userID, "imported-utm.example.com")
	if err != nil {
		t.Fatal(err)
	}
	otherSite, err := store.CreateSite(ctx, userID, "other-imported-utm.example.com")
	if err != nil {
		t.Fatal(err)
	}

	partialDay := time.Date(2026, time.April, 8, 0, 0, 0, 0, time.UTC)
	split := partialDay.Add(12 * time.Hour)
	currentDay, previousDay := partialDay.AddDate(0, 0, 1), partialDay.AddDate(0, 0, -1)
	noDetailDay, ambiguousDetailDay := partialDay.AddDate(0, 0, -10), partialDay.AddDate(0, 0, -14)
	writeImport := func(siteID uuid.UUID, days []time.Time, rows []importables.DimensionRow) {
		t.Helper()
		sink, err := NewImportedDataSink(ctx, store, siteID, uuid.New())
		if err != nil {
			t.Fatal(err)
		}
		defer sink.Abort()
		for _, day := range days {
			if err := sink.PutTraffic(ctx, importables.TrafficRow{
				Date: day, Visitors: 20, Visits: 20, Pageviews: 80, SourceFile: "imported_visitors.csv",
			}); err != nil {
				t.Fatal(err)
			}
		}
		for _, row := range rows {
			row.Visitors, row.Visits, row.SourceFile = 1, 1, "imported_sources.csv"
			if err := sink.PutDimension(ctx, row); err != nil {
				t.Fatal(err)
			}
		}
		if err := sink.Flush(ctx); err != nil {
			t.Fatal(err)
		}
	}

	dimensions := []importables.DimensionRow{
		{Date: currentDay, Dimension: "utm_campaign", Name: "spring", Pageviews: 3},
		{Date: currentDay, Dimension: "utm_content", Name: "banner", Pageviews: 5},
		{Date: currentDay, Dimension: "utm_medium", Name: "cpc", Pageviews: 7},
		{Date: currentDay, Dimension: "utm_source", Name: "search", Pageviews: 4},
		{Date: currentDay, Dimension: "utm_term", Name: "analytics", Pageviews: 9},
		{Date: previousDay, Dimension: "utm_campaign", Name: "winter", Pageviews: 11},
		{Date: previousDay, Dimension: "utm_content", Name: "text", Pageviews: 13},
		{Date: previousDay, Dimension: "utm_medium", Name: "email", Pageviews: 15},
		{Date: previousDay, Dimension: "utm_source", Name: "newsletter", Pageviews: 17},
		{Date: previousDay, Dimension: "utm_term", Name: "reporting", Pageviews: 19},
		{Date: partialDay, Dimension: "utm_source", Name: "partial day", Pageviews: 50},
		{Date: noDetailDay, Dimension: "source", Name: "news.example.com", Pageviews: 31},
		// Plausible joins nonempty fields, so the positions of these values are lost.
		{Date: ambiguousDetailDay, Dimension: "source", Name: "search.example.com", Detail: "search | cpc | spring", Pageviews: 37},
	}
	for i := range 11 {
		dimensions = append(dimensions, importables.DimensionRow{
			Date: currentDay, Dimension: "utm_source", Name: fmt.Sprintf("partner-%d", i), Pageviews: 2,
		})
	}
	writeImport(site.ID, []time.Time{currentDay, previousDay, partialDay, noDetailDay, ambiguousDetailDay}, dimensions)
	writeImport(otherSite.ID, []time.Time{currentDay, previousDay}, []importables.DimensionRow{
		{Date: currentDay, Dimension: "utm_source", Name: "search", Pageviews: 41},
		{Date: previousDay, Dimension: "utm_source", Name: "newsletter", Pageviews: 43},
	})
	if err := store.CreateHitsBulk(ctx, []*api.Hit{
		{
			SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: split.Add(15 * time.Minute), Path: "/landing",
			UTMCampaign: new("spring"), UTMContent: new("banner"), UTMMedium: new("cpc"),
			UTMSource: new("search"), UTMTerm: new("analytics"), PaidClick: new(true),
		},
		{
			SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: split.Add(-15 * time.Minute), Path: "/landing",
			UTMMedium: new("email"), UTMSource: new("newsletter"), PaidClick: new(false),
		},
	}); err != nil {
		t.Fatal(err)
	}

	params := api.AnalyticsParams{
		SiteID: site.ID, Start: split, End: split.Add(72 * time.Hour),
		CompareStart: split.Add(-72 * time.Hour), CompareEnd: split.Add(-time.Millisecond),
	}
	filtered := params
	filtered.Filters = []api.Filter{{Type: "path", Value: "/landing"}}
	subday := params
	// Both windows still contain a complete imported day at sub-day chart grain.
	subday.End = split.Add(36*time.Hour - time.Millisecond)
	subday.CompareStart = split.Add(-36 * time.Hour)
	sourceOnly := api.AnalyticsParams{
		SiteID: site.ID, Start: noDetailDay, End: noDetailDay.AddDate(0, 0, 3).Add(-time.Millisecond),
		CompareStart: ambiguousDetailDay, CompareEnd: ambiguousDetailDay.AddDate(0, 0, 3).Add(-time.Millisecond),
	}

	type campaignKPIs struct {
		campaign, content, medium, source, term int
		paid, measured                          int
	}
	nativeCurrent := campaignKPIs{campaign: 1, content: 1, medium: 1, source: 1, term: 1, paid: 1, measured: 1}
	nativePrevious := campaignKPIs{medium: 1, source: 1, measured: 1}
	blendedCurrent := campaignKPIs{campaign: 4, content: 6, medium: 8, source: 27, term: 10, paid: 1, measured: 1}
	blendedPrevious := campaignKPIs{campaign: 11, content: 13, medium: 16, source: 18, term: 19, measured: 1}
	const unattributedSourceRows = "source rows without explicit UTM dimensions"
	for _, tc := range []struct {
		name                  string
		params                api.AnalyticsParams
		sections              SiteStatsSections
		wantCurrent, wantPrev campaignKPIs
	}{
		{"KPIs only", params, SiteStatsSections{}, blendedCurrent, blendedPrevious},
		{"top lists", params, SiteStatsSections{TopLists: true}, blendedCurrent, blendedPrevious},
		{"filtered", filtered, SiteStatsSections{}, nativeCurrent, nativePrevious},
		{"subday", subday, SiteStatsSections{}, nativeCurrent, nativePrevious},
		{unattributedSourceRows, sourceOnly, SiteStatsSections{}, campaignKPIs{}, campaignKPIs{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats, err := store.GetSiteStatsSections(t.Context(), tc.params, tc.sections)
			if err != nil {
				t.Fatal(err)
			}
			gotCurrent := campaignKPIs{
				stats.UTMCampaignHits, stats.UTMContentHits, stats.UTMMediumHits, stats.UTMSourceHits, stats.UTMTermHits,
				stats.PaidSignalSessions, stats.PaidSignalMeasuredSessions,
			}
			if gotCurrent != tc.wantCurrent {
				t.Errorf("current KPIs = %+v, want %+v", gotCurrent, tc.wantCurrent)
			}
			prev := stats.Comparison
			if prev == nil {
				t.Fatal("missing comparison stats")
			}
			gotPrev := campaignKPIs{
				prev.UTMCampaignHits, prev.UTMContentHits, prev.UTMMediumHits, prev.UTMSourceHits, prev.UTMTermHits,
				prev.PaidSignalSessions, prev.PaidSignalMeasuredSessions,
			}
			if gotPrev != tc.wantPrev {
				t.Errorf("comparison KPIs = %+v, want %+v", gotPrev, tc.wantPrev)
			}
			if tc.name == unattributedSourceRows && (stats.TotalPageviews != 80 || prev.TotalPageviews != 80) {
				t.Errorf("source-only pageviews = %d current, %d comparison, want 80 each", stats.TotalPageviews, prev.TotalPageviews)
			}
			if tc.sections.TopLists {
				if len(stats.TopUTMSources) != 10 || !containsMetric(stats.TopUTMSources, "search", 5) {
					t.Errorf("expected ten top sources including search with five pageviews, got %+v", stats.TopUTMSources)
				}
			} else if len(stats.TopUTMSources) != 0 {
				t.Errorf("KPI-only request returned UTM sources: %+v", stats.TopUTMSources)
			}
		})
	}
}
