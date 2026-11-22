package database

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/importables"
)

func TestImportedSinkSkipsNativeTrafficOverlap(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "traffic-overlap.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	overlapDay := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: overlapDay,
		Path:      "/native",
	}); err != nil {
		t.Fatalf("create native hit: %v", err)
	}

	plan, err := store.BuildImportOverlapPlan(ctx, site.ID, overlapDay, overlapDay.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("build overlap plan: %v", err)
	}
	sink, err := NewImportedDataSinkWithOptions(ctx, store, site.ID, uuid.New(), ImportedDataSinkOptions{Overlap: plan})
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: overlapDay, Visitors: 2, Visits: 2, Pageviews: 7, SourceFile: "imported_visitors.csv"}); err != nil {
		t.Fatalf("put overlapped traffic: %v", err)
	}
	clearDay := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: clearDay, Visitors: 3, Visits: 3, Pageviews: 9, SourceFile: "imported_visitors.csv"}); err != nil {
		t.Fatalf("put clear traffic: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	var pageviews int
	if err := store.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(pageviews), 0) FROM imported_traffic_daily WHERE site_id = ?`, site.ID).Scan(&pageviews); err != nil {
		t.Fatalf("query imported pageviews: %v", err)
	}
	if pageviews != 9 {
		t.Fatalf("expected only non-overlapping imported pageviews, got %d", pageviews)
	}
}

func TestImportedSinkSkipsNativeEventOverlap(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "event-overlap.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	overlapDay := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		Name:      "signup",
		Timestamp: overlapDay,
	}); err != nil {
		t.Fatalf("create native event: %v", err)
	}

	plan, err := store.BuildImportOverlapPlan(ctx, site.ID, overlapDay, overlapDay.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("build overlap plan: %v", err)
	}
	sink, err := NewImportedDataSinkWithOptions(ctx, store, site.ID, uuid.New(), ImportedDataSinkOptions{Overlap: plan})
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: overlapDay, EventName: "signup", Visitors: 2, Events: 4, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put overlapped event: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: overlapDay, EventName: "purchase", Visitors: 1, Events: 1, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put clear same-day event: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: overlapDay.AddDate(0, 0, 1), EventName: "signup", Visitors: 3, Events: 3, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put clear next-day event: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	var events int
	if err := store.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(events), 0) FROM imported_event_daily WHERE site_id = ?`, site.ID).Scan(&events); err != nil {
		t.Fatalf("query imported events: %v", err)
	}
	if events != 4 {
		t.Fatalf("expected non-overlapping imported event totals only, got %d", events)
	}
}

func TestEventGoalsIncludeImportedAggregateVisitors(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "imported-goal.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.CreateGoal(ctx, &api.Goal{SiteID: site.ID, Name: "Signup", Type: "event", Value: "signup"}); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: day, Visitors: 10, Visits: 10, Pageviews: 20, SourceFile: "imported_visitors.csv"}); err != nil {
		t.Fatalf("put traffic: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: day, EventName: "signup", Visitors: 3, Events: 8, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put event: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{SiteID: site.ID, Start: day, End: day.AddDate(0, 0, 7)})
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if len(stats.Goals) != 1 || stats.Goals[0].Conversions != 3 {
		t.Fatalf("expected imported visitors to count as event goal conversions, got %+v", stats.Goals)
	}
}

func TestSiteStatsMixedGoalConversions(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := t.Context()
	site, err := store.CreateSite(ctx, userID, "mixed-goals.example.com")
	if err != nil {
		t.Fatal(err)
	}
	firstDay := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	goals := []api.Goal{
		{Name: "Signup", Type: "event", Value: "signup"},
		{Name: "Signup page", Type: "path", Value: "/signup"},
		{Name: "Checkout", Type: "event", Value: "check'out"},
		{Name: "Signup duplicate", Type: "event", Value: "signup"},
		{Name: "Signup page duplicate", Type: "path", Value: "/signup"},
		{Name: "Missing page", Type: "path", Value: "/missing"},
		{Name: "Missing event", Type: "event", Value: "missing"},
	}
	for i := range goals {
		goals[i].SiteID = site.ID
		goals[i].CreatedAt = firstDay.Add(-time.Duration(i) * time.Second)
		if err := store.CreateGoal(ctx, &goals[i]); err != nil {
			t.Fatal(err)
		}
	}

	// June 2 and 4 are complete comparison days. June 6 and 7 are complete current days.
	for _, offset := range []int{0, 1, 3, 4, 5, 6, 7} {
		putImportedComparisonRows(t, ctx, store, site.ID, firstDay.AddDate(0, 0, offset))
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO imported_event_daily (site_id, import_id, date, event_name, visitors, source_file)
		SELECT ?, uuid(), DATE '2026-06-07', ?, visitors, 'fixture' FROM (VALUES (1), (2)) AS rows(visitors)`,
		site.ID, "check'out"); err != nil {
		t.Fatal(err)
	}

	for _, visit := range []struct {
		offset int
		paths  []string
		events []string
	}{
		{2, []string{"/pricing", "/signup", "/signup"}, []string{"signup", "signup"}},
		{2, []string{"/other"}, []string{"check'out"}},
		{4, []string{"/pricing", "/signup", "/signup"}, []string{"signup", "signup"}},
		{4, []string{"/signup"}, []string{"signup"}},
		{4, []string{"/pricing"}, []string{"check'out"}},
	} {
		session, at := uuid.New(), firstDay.AddDate(0, 0, visit.offset).Add(13*time.Hour)
		for i, path := range visit.paths {
			hit := api.Hit{SiteID: site.ID, SessionID: session, PageID: uuid.New(), Timestamp: at.Add(time.Duration(i) * time.Minute), Path: path}
			if err := store.CreateHit(ctx, &hit); err != nil {
				t.Fatal(err)
			}
		}
		for i, name := range visit.events {
			event := api.Event{SiteID: site.ID, SessionID: session, Timestamp: at.Add(time.Duration(i) * time.Minute), Name: name}
			if err := store.CreateEvent(ctx, &event); err != nil {
				t.Fatal(err)
			}
		}
	}

	params := api.AnalyticsParams{
		SiteID:       site.ID,
		Start:        firstDay.AddDate(0, 0, 4).Add(12 * time.Hour),
		End:          firstDay.AddDate(0, 0, 7).Add(12 * time.Hour),
		CompareStart: firstDay.Add(12 * time.Hour),
		CompareEnd:   firstDay.AddDate(0, 0, 4).Add(-time.Millisecond),
	}
	assertGoals := func(t *testing.T, got []api.GoalStats, counts []int, sessions int) {
		t.Helper()
		want := make([]api.GoalStats, len(goals))
		for i, goal := range goals {
			want[i] = api.GoalStats{GoalID: goal.ID, Name: goal.Name, Conversions: counts[i], ConversionRate: (float64(counts[i]) / float64(sessions)) * 100}
		}
		if !slices.Equal(got, want) {
			t.Errorf("goals = %+v, want %+v", got, want)
		}
	}
	for _, tc := range []struct {
		name                          string
		filters                       []api.Filter
		current, previous             []int
		currentSessions, prevSessions int
		previousTotal                 int
	}{
		{"with complete imported days", nil, []int{10, 2, 4, 10, 2, 0, 0}, []int{9, 1, 1, 9, 1, 0, 0}, 23, 22, 21},
		{"filtered session cohort", []api.Filter{{Type: "path", Value: "/pricing"}}, []int{1, 1, 1, 1, 1, 0, 0}, []int{1, 1, 0, 1, 1, 0, 0}, 2, 1, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params.Filters = tc.filters
			stats := getComparisonDashboardStats(t, t.Context(), store, params)
			if stats.Comparison == nil {
				t.Fatal("missing comparison")
			}
			if stats.UniqueSessions != tc.currentSessions || stats.Comparison.UniqueSessions != tc.prevSessions {
				t.Errorf("sessions = %d current and %d previous, want %d and %d", stats.UniqueSessions, stats.Comparison.UniqueSessions, tc.currentSessions, tc.prevSessions)
			}
			assertGoals(t, stats.Goals, tc.current, tc.currentSessions)
			assertGoals(t, stats.Comparison.Goals, tc.previous, tc.prevSessions)
			if stats.Comparison.TotalConversions != tc.previousTotal {
				t.Errorf("previous total conversions = %d, want %d", stats.Comparison.TotalConversions, tc.previousTotal)
			}
		})
	}
}

func TestImportedAggregatesIncludedWithComparisonRequest(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := t.Context()
	site, err := store.CreateSite(ctx, userID, "imported-comparison.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	goal := api.Goal{SiteID: site.ID, Name: "Signup", Type: "event", Value: "signup"}
	if err := store.CreateGoal(ctx, &goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	funnel := api.Funnel{SiteID: site.ID, Name: "Signup", Steps: []api.FunnelStep{{Type: "path", Value: "/pricing"}, {Type: "event", Value: "signup"}}}
	if err := store.CreateFunnel(ctx, &funnel); err != nil {
		t.Fatalf("create funnel: %v", err)
	}

	// Both windows contain a complete imported day, a native visit, and only
	// part of the days at either end. The split day is partial in both windows.
	split := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	for _, offset := range []int{-3, -2, 0, 1, 3} {
		putImportedComparisonRows(t, ctx, store, site.ID, split.AddDate(0, 0, offset))
	}
	for _, offset := range []int{-1, 2} {
		day := split.AddDate(0, 0, offset)
		session := uuid.New()
		for _, at := range []time.Time{day, day.Add(time.Minute)} {
			if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: session, PageID: uuid.New(), Timestamp: at, Path: "/pricing"}); err != nil {
				t.Fatalf("create native hit: %v", err)
			}
		}
		if err := store.CreateEvent(ctx, &api.Event{SiteID: site.ID, SessionID: session, Timestamp: day, Name: "signup"}); err != nil {
			t.Fatalf("create native event: %v", err)
		}
	}

	params := api.AnalyticsParams{
		SiteID:       site.ID,
		Start:        split,
		End:          split.Add(72*time.Hour - time.Millisecond),
		CompareStart: split.Add(-72 * time.Hour),
		CompareEnd:   split.Add(-time.Millisecond),
	}
	stats := getComparisonDashboardStats(t, ctx, store, params)
	assertComparedDashboardTotals(t, stats, 22, 11, 5, 60.0/11)
	if !containsMetric(stats.TopPages, "/pricing", 8) {
		t.Fatalf("expected current-period page breakdown to use the same import dates, got %+v", stats.TopPages)
	}
	if stats.BounceRate != (2.0/float64(stats.UniqueSessions))*100 || stats.PagesPerSession != 2 {
		t.Fatalf("expected blended bounce and pages per session, got %v and %v", stats.BounceRate, stats.PagesPerSession)
	}

	for _, test := range []struct {
		name   string
		filter func(*api.AnalyticsParams)
	}{
		{"hit filter", func(p *api.AnalyticsParams) { p.Filters = []api.Filter{{Type: "path", Value: "/pricing"}} }},
		{"goal cohort", func(p *api.AnalyticsParams) { p.GoalIDs = []uuid.UUID{goal.ID} }},
		{"funnel cohort", func(p *api.AnalyticsParams) { p.FunnelIDs = []uuid.UUID{funnel.ID} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			scoped := params
			test.filter(&scoped)
			filtered := getComparisonDashboardStats(t, t.Context(), store, scoped)
			assertComparedDashboardTotals(t, filtered, 2, 1, 1, 60)
			if len(filtered.ImportedExcluded) != 1 || filtered.ImportedExcluded[0].Reason != "aggregate_filter" {
				t.Fatalf("missing aggregate filter exclusion: %+v", filtered.ImportedExcluded)
			}
		})
	}

	params.Start, params.End = split.Add(time.Hour), split.Add(2*time.Hour-time.Millisecond)
	params.CompareStart, params.CompareEnd = split, split.Add(time.Hour-time.Millisecond)
	short := getComparisonDashboardStats(t, ctx, store, params)
	assertComparedDashboardTotals(t, short, 0, 0, 0, 0)
	if len(short.ImportedExcluded) != 1 || short.ImportedExcluded[0].Reason != "daily_grain" {
		t.Fatalf("missing daily grain exclusion: %+v", short.ImportedExcluded)
	}
}

func TestSiteStatsChartIncludesHitsAtInclusivePeriodEnds(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := t.Context()
	site, err := store.CreateSite(ctx, userID, "comparison-ends.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	previousEnd := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	currentStart := previousEnd.Add(time.Millisecond)
	currentEnd := currentStart.Add(8 * time.Hour)
	for _, at := range []time.Time{previousEnd, currentEnd} {
		if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: at, Path: "/"}); err != nil {
			t.Fatalf("create hit at %s: %v", at, err)
		}
	}

	stats := getComparisonDashboardStats(t, ctx, store, api.AnalyticsParams{
		SiteID:       site.ID,
		Start:        currentStart,
		End:          currentEnd,
		CompareStart: previousEnd.Add(-currentEnd.Sub(currentStart)),
		CompareEnd:   previousEnd,
	})
	if stats.Comparison == nil || totalChartPageviews(stats.ChartData) != 1 || totalChartPageviews(stats.Comparison.ChartData) != 1 {
		t.Fatalf("expected one chart hit in each period: current %+v, previous %+v", stats.ChartData, stats.Comparison)
	}
}

func TestSiteOverviewAndDashboardIncludeOnlyCompleteUTCImportDates(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := t.Context()
	site, err := store.CreateSite(ctx, userID, "imported-overview.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	first := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 0, 3)
	end := last.AddDate(0, 0, 1).Add(-time.Millisecond)
	partialStart, partialEnd := first.Add(12*time.Hour), last.Add(12*time.Hour)
	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	for i := range 4 {
		if err := sink.PutTraffic(ctx, importables.TrafficRow{
			Date: first.AddDate(0, 0, i), Pageviews: 10 * int64(i+1), Visits: 2 * int64(i+1),
			Visitors: int64(i + 1), Bounces: int64(i + 1), SourceFile: "datapoints.csv",
		}); err != nil {
			t.Fatalf("put imported traffic: %v", err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush imports: %v", err)
	}
	for _, at := range []time.Time{
		first.Add(-time.Millisecond), partialStart.Add(-time.Millisecond), partialStart,
		partialEnd, partialEnd.Add(time.Millisecond), end.Add(time.Millisecond),
	} {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: at, Path: "/",
		}); err != nil {
			t.Fatalf("create native hit: %v", err)
		}
	}

	for _, tc := range []struct {
		name                         string
		start, end                   time.Time
		pageviews, sessions, bounces int
		bucketPageviews              [4]int
		bucketVisitors               [4]int
	}{
		{"partial days", partialStart, partialEnd, 52, 12, 7, [4]int{1, 20, 30, 1}, [4]int{1, 2, 3, 1}},
		{"whole days", first, end, 104, 24, 14, [4]int{12, 20, 30, 42}, [4]int{3, 2, 3, 6}},
		{"before cutoff", first, end.Add(-time.Millisecond), 64, 16, 10, [4]int{12, 20, 30, 2}, [4]int{3, 2, 3, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			params := api.AnalyticsParams{SiteID: site.ID, UserID: userID, Start: tc.start, End: tc.end}
			dashboard, err := store.GetSiteStats(ctx, params)
			if err != nil {
				t.Fatalf("get dashboard stats: %v", err)
			}
			overview, err := store.GetSiteOverviewStats(ctx, params)
			if err != nil {
				t.Fatalf("get overview stats: %v", err)
			}
			wantBounceRate := float64(tc.bounces) / float64(tc.sessions) * 100
			if dashboard.TotalPageviews != tc.pageviews || dashboard.UniqueSessions != tc.sessions || dashboard.BounceRate != wantBounceRate {
				t.Errorf("dashboard KPIs = %d/%d/%v, want %d/%d/%v",
					dashboard.TotalPageviews, dashboard.UniqueSessions, dashboard.BounceRate, tc.pageviews, tc.sessions, wantBounceRate)
			}
			if overview.TotalPageviews != tc.pageviews || overview.UniqueSessions != tc.sessions || overview.BounceRate != wantBounceRate {
				t.Errorf("overview KPIs = %d/%d/%v, want %d/%d/%v",
					overview.TotalPageviews, overview.UniqueSessions, overview.BounceRate, tc.pageviews, tc.sessions, wantBounceRate)
			}
			for _, result := range []struct {
				name  string
				chart []api.ChartDataPoint
			}{{"dashboard", dashboard.ChartData}, {"overview", overview.ChartData}} {
				if len(result.chart) != len(tc.bucketPageviews) {
					t.Errorf("%s chart has %d points, want %d", result.name, len(result.chart), len(tc.bucketPageviews))
					continue
				}
				for i, point := range result.chart {
					wantDay := first.AddDate(0, 0, i)
					if !point.Time.Equal(wantDay) || point.Pageviews != tc.bucketPageviews[i] || point.Visitors != tc.bucketVisitors[i] {
						t.Errorf("%s chart point = %+v, want %s with %d pageviews and %d visitors",
							result.name, point, wantDay, tc.bucketPageviews[i], tc.bucketVisitors[i])
					}
				}
			}
		})
	}
}

func TestImportedSiteStatsIncludeGeoNetworkDimensions(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "imported-geo-network.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	day := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: day, Visitors: 12, Visits: 12, Pageviews: 24, SourceFile: "datapoints.csv"}); err != nil {
		t.Fatalf("put traffic: %v", err)
	}
	for _, row := range []importables.DimensionRow{
		{Date: day, Dimension: "city", Name: "Dortmund", Visitors: 7, Visits: 7, Pageviews: 14, SourceFile: "datapoints.csv"},
		{Date: day, Dimension: "provider", Name: "Deutsche Telekom AG", Visitors: 6, Visits: 6, Pageviews: 12, SourceFile: "datapoints.csv"},
		{Date: day, Dimension: "asn", Name: "AS3320 Deutsche Telekom AG", Visitors: 5, Visits: 5, Pageviews: 10, SourceFile: "datapoints.csv"},
	} {
		if err := sink.PutDimension(ctx, row); err != nil {
			t.Fatalf("put dimension %s: %v", row.Dimension, err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{SiteID: site.ID, Start: day, End: day.AddDate(0, 0, 7)})
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if !containsMetric(stats.TopCities, "Dortmund", 14) {
		t.Fatalf("expected imported city aggregate, got %+v", stats.TopCities)
	}
	if !containsMetric(stats.TopProviders, "Deutsche Telekom AG", 12) {
		t.Fatalf("expected imported provider aggregate, got %+v", stats.TopProviders)
	}
	if !containsMetric(stats.TopASNs, "AS3320 Deutsche Telekom AG", 10) {
		t.Fatalf("expected imported ASN aggregate, got %+v", stats.TopASNs)
	}
}

func putImportedComparisonRows(t *testing.T, ctx context.Context, store *Store, siteID uuid.UUID, day time.Time) {
	t.Helper()
	sink, err := NewImportedDataSink(ctx, store, siteID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: day, Visitors: 10, Visits: 10, Pageviews: 20, Bounces: 2, SourceFile: "datapoints.csv"}); err != nil {
		t.Fatalf("put traffic: %v", err)
	}
	if err := sink.PutDimension(ctx, importables.DimensionRow{Date: day, Dimension: "page", Name: "/pricing", Visitors: 3, Visits: 3, Pageviews: 6, SourceFile: "datapoints.csv"}); err != nil {
		t.Fatalf("put dimension: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: day, EventName: "signup", Visitors: 4, Events: 7, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put event: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
}

func getComparisonDashboardStats(t *testing.T, ctx context.Context, store *Store, params api.AnalyticsParams) *api.SiteStats {
	t.Helper()
	stats, err := store.GetSiteStats(ctx, params)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	return stats
}

func assertComparedDashboardTotals(t *testing.T, stats *api.SiteStats, pageviews, sessions, conversions int, duration float64) {
	t.Helper()
	previous := stats.Comparison
	if previous == nil {
		t.Fatal("expected comparison stats")
	}
	if stats.TotalPageviews != pageviews || previous.TotalPageviews != pageviews ||
		stats.UniqueSessions != sessions || previous.UniqueSessions != sessions ||
		stats.AvgSessionDuration != duration || previous.AvgSessionDuration != duration ||
		stats.BounceRate != previous.BounceRate || stats.PagesPerSession != previous.PagesPerSession {
		t.Fatalf("unexpected period KPIs: current %+v, previous %+v", stats, previous)
	}
	if totalChartPageviews(stats.ChartData) != pageviews || totalChartPageviews(previous.ChartData) != pageviews {
		t.Fatalf("unexpected chart counts: current %+v, previous %+v", stats.ChartData, previous.ChartData)
	}
	if len(stats.Goals) != 1 || len(previous.Goals) != 1 ||
		stats.Goals[0].Conversions != conversions || stats.Goals[0] != previous.Goals[0] || previous.TotalConversions != conversions {
		t.Fatalf("unexpected goal comparison: current %+v, previous %+v", stats.Goals, previous.Goals)
	}
}

func totalChartPageviews(points []api.ChartDataPoint) int {
	var pageviews int
	for _, point := range points {
		pageviews += point.Pageviews
	}
	return pageviews
}

func TestEventAudienceReportsMissingImportedDimensions(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "imported-audience.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := sink.PutEvent(ctx, importables.EventRow{Date: day, EventName: "signup", Path: "/pricing", Visitors: 3, Events: 8, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put event: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	audience, err := store.GetEventAudience(ctx, api.EventAudienceParams{SiteID: site.ID, Start: day, End: day.AddDate(0, 0, 7), EventName: "signup"})
	if err != nil {
		t.Fatalf("get audience: %v", err)
	}
	if len(audience.TopPages) != 1 || audience.TopPages[0].Name != "/pricing" || audience.TopPages[0].Value != 3 {
		t.Fatalf("expected imported path audience, got %+v", audience.TopPages)
	}
	if len(audience.ImportedExcluded) != 1 || audience.ImportedExcluded[0].Reason != "missing_event_dimension_relationships" {
		t.Fatalf("expected missing dimension limitation, got %+v", audience.ImportedExcluded)
	}
}

func TestEventAudienceIncludesImportedEventDimensions(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "imported-audience-dimensions.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := sink.PutEvent(ctx, importables.EventRow{Date: day, EventName: "signup", Path: "/pricing", Visitors: 3, Events: 8, SourceFile: "imported_events.csv"}); err != nil {
		t.Fatalf("put event: %v", err)
	}
	for _, row := range []importables.EventDimensionRow{
		{Date: day, EventName: "signup", Dimension: "path", Name: "/pricing", Visitors: 3, Events: 8, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "referrer", Name: "Google", Visitors: 2, Events: 5, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "device", Name: "Desktop", Visitors: 2, Events: 5, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "country", Name: "Germany", Visitors: 1, Events: 3, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "city", Name: "Berlin", Visitors: 1, Events: 3, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "provider", Name: "Hetzner Online GmbH", Visitors: 1, Events: 3, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "asn", Name: "AS24940 Hetzner Online GmbH", Visitors: 1, Events: 3, SourceFile: "imported_event_dimensions.csv"},
	} {
		if err := sink.PutEventDimension(ctx, row); err != nil {
			t.Fatalf("put event dimension %s: %v", row.Dimension, err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	audience, err := store.GetEventAudience(ctx, api.EventAudienceParams{SiteID: site.ID, Start: day, End: day.AddDate(0, 0, 7), EventName: "signup"})
	if err != nil {
		t.Fatalf("get audience: %v", err)
	}
	if len(audience.TopPages) != 1 || audience.TopPages[0].Name != "/pricing" || audience.TopPages[0].Value != 3 {
		t.Fatalf("expected imported path audience, got %+v", audience.TopPages)
	}
	if len(audience.TopReferrers) != 1 || audience.TopReferrers[0].Name != "Google" || audience.TopReferrers[0].Value != 2 {
		t.Fatalf("expected imported referrer audience, got %+v", audience.TopReferrers)
	}
	if len(audience.TopDevices) != 1 || audience.TopDevices[0].Name != "Desktop" || audience.TopDevices[0].Value != 2 {
		t.Fatalf("expected imported device audience, got %+v", audience.TopDevices)
	}
	if len(audience.TopCountries) != 1 || audience.TopCountries[0].Name != "Germany" || audience.TopCountries[0].Value != 1 {
		t.Fatalf("expected imported country audience, got %+v", audience.TopCountries)
	}
	if len(audience.TopCities) != 1 || audience.TopCities[0].Name != "Berlin" || audience.TopCities[0].Value != 1 {
		t.Fatalf("expected imported city audience, got %+v", audience.TopCities)
	}
	if len(audience.TopProviders) != 1 || audience.TopProviders[0].Name != "Hetzner Online GmbH" || audience.TopProviders[0].Value != 1 {
		t.Fatalf("expected imported provider audience, got %+v", audience.TopProviders)
	}
	if len(audience.TopASNs) != 1 || audience.TopASNs[0].Name != "AS24940 Hetzner Online GmbH" || audience.TopASNs[0].Value != 1 {
		t.Fatalf("expected imported ASN audience, got %+v", audience.TopASNs)
	}
	if len(audience.ImportedExcluded) != 0 {
		t.Fatalf("expected no missing-dimension limitation when aggregate dimensions exist, got %+v", audience.ImportedExcluded)
	}
}

func TestDeleteImportedDataForImportWithUnattributedProperties(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "import-delete-properties.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	importID := uuid.New()
	sink, err := NewImportedDataSink(ctx, store, site.ID, importID)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	dates := []time.Time{
		time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
	}
	for _, date := range dates {
		if err := sink.PutEventProperty(ctx, importables.EventPropertyRow{
			Date:          date,
			EventName:     "outbound_click",
			PropertyKey:   "url",
			PropertyValue: "https://example.com",
			Visitors:      1,
			Events:        1,
			SourceFile:    "imported_custom_events.csv",
		}); err != nil {
			t.Fatalf("put attributed property: %v", err)
		}
		if err := sink.PutEventProperty(ctx, importables.EventPropertyRow{
			Date:          date,
			PropertyKey:   "url",
			PropertyValue: "https://example.com",
			Visitors:      1,
			Events:        1,
			SourceFile:    "imported_unattributed_properties.csv",
		}); err != nil {
			t.Fatalf("put unattributed property: %v", err)
		}
		if err := sink.PutEventDimension(ctx, importables.EventDimensionRow{Date: date, EventName: "outbound_click", Dimension: "url", Name: "https://example.com", Visitors: 1, Events: 1, SourceFile: "imported_event_dimensions.csv"}); err != nil {
			t.Fatalf("put event dimension: %v", err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	var nullEventNames int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM imported_event_properties_daily WHERE site_id = ? AND event_name IS NULL`, site.ID).Scan(&nullEventNames); err != nil {
		t.Fatalf("query null imported property event names: %v", err)
	}
	if nullEventNames != 0 {
		t.Fatalf("expected unattributed properties to use non-null event name sentinel, got %d null rows", nullEventNames)
	}

	if _, err := store.DeleteImportedDataForImport(ctx, site.ID, importID); err != nil {
		t.Fatalf("delete imported data: %v", err)
	}
	var remaining int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM imported_event_properties_daily WHERE site_id = ?`, site.ID).Scan(&remaining); err != nil {
		t.Fatalf("query remaining imported properties after delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected imported property rows to be deleted, got %d", remaining)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM imported_event_dimensions_daily WHERE site_id = ?`, site.ID).Scan(&remaining); err != nil {
		t.Fatalf("query remaining imported dimensions after delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected imported event dimension rows to be deleted, got %d", remaining)
	}
}
