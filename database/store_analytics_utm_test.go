package database

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/importables"
)

func TestUTMOverviewRanksTaggedPageviews(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := t.Context()
	site, err := store.CreateSite(ctx, userID, "utm-rankings.example")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	params := api.AnalyticsParams{SiteID: site.ID, Start: start, End: start.AddDate(0, 0, 5), UTMOverview: true}
	var hits []*api.Hit
	add := func(hit api.Hit, count int) {
		for range count {
			row := hit
			row.SiteID, row.SessionID, row.PageID = site.ID, uuid.New(), uuid.New()
			row.Timestamp, row.Path = start.Add(time.Hour), "/landing"
			hits = append(hits, &row)
		}
	}
	add(api.Hit{}, 50)
	add(api.Hit{UTMSource: new(" "), UTMMedium: new("\t"), UTMCampaign: new(""), UTMContent: new("\r\n"), UTMTerm: new("  ")}, 5)
	add(api.Hit{UTMSource: new(" unspecified "), UTMMedium: new("email"), UTMCampaign: new("launch"), UTMContent: new("hero"), UTMTerm: new("analytics")}, 3)
	add(api.Hit{UTMSource: new("(Unspecified)"), UTMMedium: new("email")}, 2)
	for i := range 10 {
		add(api.Hit{UTMSource: new(fmt.Sprintf("partner-%02d", i))}, 2)
	}
	add(api.Hit{UTMSource: new("late")}, 1)
	if err := store.CreateHitsBulk(ctx, hits); err != nil {
		t.Fatal(err)
	}
	// A source below the native cutoff becomes first when eligible history is included.
	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Abort()
	day := start.AddDate(0, 0, 2)
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: day, Pageviews: 45, Visits: 45, Visitors: 45, SourceFile: "traffic.csv"}); err != nil {
		t.Fatal(err)
	}
	for name, count := range map[string]int64{"late": 5, "   ": 40} {
		if err := sink.PutDimension(ctx, importables.DimensionRow{Date: day, Dimension: "utm_source", Name: name, Pageviews: count, SourceFile: "sources.csv"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	legacy := params
	legacy.UTMOverview = false
	all, err := store.GetSiteStats(ctx, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !containsMetric(all.TopUTMSources, "(Unspecified)", 57) || all.TotalPageviews != 126 {
		t.Fatalf("default overview: pageviews %d, UTM sources %+v", all.TotalPageviews, all.TopUTMSources)
	}

	stats, err := store.GetSiteStats(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	var chartViews int
	for _, point := range stats.ChartData {
		chartViews += point.Pageviews
	}
	if stats.TotalPageviews != 126 || chartViews != 126 || stats.UTMSourceHits != 31 {
		t.Fatalf("pageviews %d, chart %d, source-tagged %d; want 126, 126, 31", stats.TotalPageviews, chartViews, stats.UTMSourceHits)
	}
	if len(stats.TopUTMSources) != 10 || stats.TopUTMSources[0] != (api.MetricStat{Name: "late", Value: 6}) ||
		!containsMetric(stats.TopUTMSources, "unspecified", 3) || !containsMetric(stats.TopUTMSources, "(Unspecified)", 2) ||
		stats.TopUTMSources[9].Name != "partner-06" {
		t.Fatalf("top ten sources = %+v", stats.TopUTMSources)
	}
	for _, dim := range []struct {
		rows  []api.MetricStat
		total int
		name  string
		want  int
	}{
		{stats.TopUTMMediums, stats.UTMMediumHits, "email", 5},
		{stats.TopUTMCampaigns, stats.UTMCampaignHits, "launch", 3},
		{stats.TopUTMContents, stats.UTMContentHits, "hero", 3},
		{stats.TopUTMTerms, stats.UTMTermHits, "analytics", 3},
	} {
		if dim.total != dim.want || !slices.Equal(dim.rows, []api.MetricStat{{Name: dim.name, Value: dim.want}}) {
			t.Errorf("%s: total %d, rows %+v", dim.name, dim.total, dim.rows)
		}
	}

	for name, want := range map[string]int{"unspecified": 3, "(Unspecified)": 2} {
		params.Filters = []api.Filter{{Type: "utm_source", Value: name}}
		filtered, err := store.GetSiteStats(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		if filtered.TotalPageviews != want || !slices.Equal(filtered.TopUTMSources, []api.MetricStat{{Name: name, Value: want}}) {
			t.Errorf("drilldown %q: pageviews %d, sources %+v", name, filtered.TotalPageviews, filtered.TopUTMSources)
		}
		if name == "(Unspecified)" && (filtered.UTMCampaignHits != 0 || len(filtered.TopUTMCampaigns) != 0) {
			t.Errorf("unexpected campaign on partially tagged pageviews: %+v", filtered.TopUTMCampaigns)
		}
	}
}
