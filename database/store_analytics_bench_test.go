package database

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

// benchmarkSiteStats is a month of traffic for one busy site: 200k hits in
// multi-page sessions across 60 paths, referrers, countries, and agents.
func benchmarkSiteStats(b *testing.B) (*Store, api.AnalyticsParams) {
	b.Helper()
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		b.Fatal(err)
	}
	userID, err := store.CreateUser(ctx, "bench@example.com", "hash")
	if err != nil {
		b.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "bench.example")
	if err != nil {
		b.Fatal(err)
	}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewPCG(1, 2))
	ids := rand.NewChaCha8([32]byte{1})
	newID := func() uuid.UUID { return uuid.Must(uuid.NewRandomFromReader(ids)) }
	referrers := []string{"", "", "https://www.google.com/", "https://news.ycombinator.com/item?id=1", "https://chatgpt.com/", "https://t.co/x"}
	agents := []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)",
	}
	countries := []string{"DE", "US", "FR", "GB", "NL"}
	hits := make([]*api.Hit, 0, 200_000)
	for len(hits) < cap(hits) {
		session, referrer, country := newID(), referrers[rng.IntN(len(referrers))], countries[rng.IntN(len(countries))]
		// 2,000 distinct agents: real traffic repeats agents, but not three.
		agent := fmt.Sprintf("%s build/%d", agents[rng.IntN(len(agents))], rng.IntN(2000))
		at := start.Add(time.Duration(rng.IntN(29*24*3600)) * time.Second)
		for page := range 1 + rng.IntN(5) {
			width := 390 + rng.IntN(3)*700
			hit := &api.Hit{
				SiteID: site.ID, SessionID: session, PageID: newID(), Timestamp: at.Add(time.Duration(page) * time.Minute),
				Path: fmt.Sprintf("/page-%d", rng.IntN(60)), UserAgent: &agent, CountryCode: &country, ViewportWidth: &width,
			}
			if page == 0 && referrer != "" {
				hit.Referrer = &referrer
			}
			hits = append(hits, hit)
		}
	}
	for chunk := range slices.Chunk(hits, 10_000) {
		if err := store.CreateHitsBulk(ctx, chunk); err != nil {
			b.Fatal(err)
		}
	}
	return store, api.AnalyticsParams{SiteID: site.ID, Start: start, End: start.AddDate(0, 0, 29)}
}

func BenchmarkGetSiteStats(b *testing.B) {
	store, params := benchmarkSiteStats(b)
	ctx := context.Background()
	for name, sections := range map[string]SiteStatsSections{
		"all":        AllSiteStatsSections,
		"kpis":       {},
		"chart":      {Chart: true},
		"top_lists":  {TopLists: true},
		"entry_exit": {EntryExit: true},
		"goals":      {Goals: true},
	} {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				if _, err := store.GetSiteStatsSections(ctx, params, sections); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkImportedUTMKPIs(b *testing.B) {
	store, params := benchmarkSiteStats(b)
	ctx := b.Context()
	if _, err := store.db.ExecContext(ctx, `
		UPDATE hits
		SET utm_campaign = 'native-campaign', utm_content = 'native-content', utm_medium = 'email',
			utm_source = 'newsletter', utm_term = 'native-term', paid_click = (path = '/page-0')
		WHERE path IN ('/page-0', '/page-1', '/page-2', '/page-3', '/page-4', '/page-5')`); err != nil {
		b.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO imported_traffic_daily (site_id, import_id, date, visitors, visits, pageviews, bounces, visit_duration, source_file)
		SELECT ?, '00000000-0000-4000-8000-000000000001', day, 900, 1000, 2500, 200, 30000, 'benchmark'
		FROM range(CAST(? AS DATE), CAST(? AS DATE), INTERVAL 1 DAY) AS dates(day)`,
		params.SiteID, params.Start, params.End); err != nil {
		b.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO imported_dimension_daily (site_id, import_id, date, dimension, name, pageviews, source_file)
		SELECT ?, '00000000-0000-4000-8000-000000000001', day, dimension, dimension || '-' || label,
			5 + (label % 20), 'benchmark'
		FROM range(CAST(? AS DATE), CAST(? AS DATE), INTERVAL 1 DAY) AS dates(day)
		CROSS JOIN (VALUES ('utm_campaign', 40), ('utm_content', 60), ('utm_medium', 8), ('utm_source', 20),
			('utm_term', 100), ('page', 60), ('entry_page', 60), ('exit_page', 60), ('source', 20),
			('browser', 12), ('device', 3), ('country', 25), ('city', 60), ('provider', 24),
			('asn', 24), ('language', 12), ('operating_system', 6)) AS dimensions(dimension, cardinality)
		CROSS JOIN LATERAL range(cardinality) AS labels(label)`, params.SiteID, params.Start, params.End); err != nil {
		b.Fatal(err)
	}

	params.CompareStart = params.Start
	params.Start = params.Start.AddDate(0, 0, 15)
	params.CompareEnd = params.Start.Add(-time.Millisecond)
	params.End = params.End.Add(-time.Millisecond)
	for _, scenario := range []struct {
		name     string
		sections SiteStatsSections
	}{
		{"dashboard", AllSiteStatsSections},
		{"kpis", SiteStatsSections{}},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			ctx := b.Context()
			run := func() *api.SiteStats {
				stats, err := store.GetSiteStatsSections(ctx, params, scenario.sections)
				if err != nil {
					b.Fatal(err)
				}
				return stats
			}
			// Warm the rollups and prepared chart reads before sampling the steady-state operation.
			stats := run()
			if stats.TotalPageviews <= 14*2500 || stats.Comparison == nil || stats.Comparison.TotalPageviews <= 15*2500 {
				b.Fatalf("fixture needs native hits in both windows")
			}
			if _, err := store.db.ExecContext(ctx, `CALL truncate_duckdb_logs()`); err != nil {
				b.Fatal(err)
			}
			if _, err := store.db.ExecContext(ctx, `CALL enable_logging('QueryLog')`); err != nil {
				b.Fatal(err)
			}
			run()
			if _, err := store.db.ExecContext(ctx, `CALL disable_logging()`); err != nil {
				b.Fatal(err)
			}
			var logEntries int
			if err := store.db.QueryRowContext(ctx, `
				SELECT COUNT(DISTINCT query_id) FROM duckdb_logs
				WHERE type = 'QueryLog' AND message NOT LIKE '%disable_logging%'`).Scan(&logEntries); err != nil {
				b.Fatal(err)
			}
			for b.Loop() {
				run()
			}
			b.ReportMetric(float64(logEntries), "query-log/op")
			b.ReportMetric(float64(stats.UTMTermHits), "current-utm-term/op")
			b.ReportMetric(float64(stats.Comparison.UTMTermHits), "comparison-utm-term/op")
		})
	}
}

func BenchmarkComparisonAndBreakdown(b *testing.B) {
	store, params := benchmarkSiteStats(b)
	ctx := context.Background()
	b.Run("comparison", func(b *testing.B) {
		compare := params
		compare.CompareStart, compare.CompareEnd = params.Start.AddDate(0, 0, -29), params.Start
		for b.Loop() {
			if _, err := store.GetComparisonStats(ctx, compare); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ai_activity", func(b *testing.B) {
		compare := params
		compare.CompareStart, compare.CompareEnd = params.Start.AddDate(0, 0, -29), params.Start
		for b.Loop() {
			if _, err := store.GetAIActivity(ctx, compare); err != nil {
				b.Fatal(err)
			}
		}
	})
	for _, dimension := range []string{"page", "browser", "ai_bot"} {
		b.Run("breakdown_"+dimension, func(b *testing.B) {
			for b.Loop() {
				if _, err := store.GetDimensionBreakdown(ctx, params, dimension, 50); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSiteGoalConversions(b *testing.B) {
	store, params := benchmarkSiteStats(b)
	ctx := b.Context()
	// Seed events across both windows and daily imports for all four event goals.
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO events (site_id, session_id, name, timestamp)
		SELECT site_id, session_id, 'event-' || (hash(session_id) % 12), timestamp
		FROM hits WHERE hash(page_id) % 5 = 0`); err != nil {
		b.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO imported_event_daily (site_id, import_id, date, event_name, visitors, source_file)
		SELECT ?, uuid(), day, 'event-' || name, 10, 'benchmark'
		FROM range(CAST(? AS DATE), CAST(? AS DATE), INTERVAL 1 DAY) AS dates(day), range(4) AS names(name)`,
		params.SiteID, params.Start, params.End); err != nil {
		b.Fatal(err)
	}

	previous := params
	params.Start = params.Start.AddDate(0, 0, 15)
	previous.End = params.Start.Add(-time.Millisecond)
	for _, size := range []int{1, 10} {
		from := 0
		if size == 10 {
			from = 1
		}
		for i := from; i < size; i++ {
			goal := api.Goal{SiteID: params.SiteID, Type: "event", Value: fmt.Sprintf("event-%d", i)}
			if i >= 4 {
				goal.Type, goal.Value = "path", fmt.Sprintf("/page-%d", i)
			}
			goal.Name = goal.Value
			if err := store.CreateGoal(ctx, &goal); err != nil {
				b.Fatal(err)
			}
		}
		for _, filtered := range []bool{false, true} {
			b.Run(fmt.Sprintf("goals=%d/filtered=%t", size, filtered), func(b *testing.B) {
				ctx := b.Context()
				windows := []api.AnalyticsParams{params, previous}
				if filtered {
					for i := range windows {
						windows[i].Filters = []api.Filter{{Type: "path", Value: "/page-0"}}
					}
				}
				filterSQL, filterArgs := buildHitFilters(windows[0].Filters, "h")
				run := func() {
					for _, window := range windows {
						if err := store.querySiteGoals(ctx, window, "day", filterSQL, filterArgs, &api.SiteStats{}); err != nil {
							b.Fatal(err)
						}
					}
				}
				if _, err := store.db.ExecContext(ctx, `CALL truncate_duckdb_logs()`); err != nil {
					b.Fatal(err)
				}
				if _, err := store.db.ExecContext(ctx, `CALL enable_logging('QueryLog')`); err != nil {
					b.Fatal(err)
				}
				run()
				if _, err := store.db.ExecContext(ctx, `CALL disable_logging()`); err != nil {
					b.Fatal(err)
				}
				var statements int
				if err := store.db.QueryRowContext(ctx, `
					SELECT COUNT(DISTINCT query_id) FROM duckdb_logs
					WHERE type = 'QueryLog' AND message NOT LIKE '%disable_logging%'`).Scan(&statements); err != nil {
					b.Fatal(err)
				}
				for b.Loop() {
					run()
				}
				// The driver logs each parameterized statement once at prepare and once at execution.
				b.ReportMetric(float64(statements)/2, "sql/op")
			})
		}
	}
}
