package database

import (
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/importables"
)

func TestReferrerHostFilter(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := t.Context()
	site, err := store.CreateSite(ctx, userID, "site.test")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, hit := range []struct{ path, referrer string }{
		{"/www-url", "https://WWW.source.test/article?q=1"},
		{"/url", "https://source.test/article"},
		{"/www-host", "www.source.test"},
		{"/host", "source.test"},
		{"/suffix", "https://www.source.test.other.test/article"},
		{"/hyphen", "www-source.test"},
	} {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(),
			Timestamp: at, Path: hit.path, Referrer: new(hit.referrer),
		}); err != nil {
			t.Fatal(err)
		}
	}

	wantHostPaths := []string{"/host", "/url", "/www-host", "/www-url"}
	for _, tc := range []struct {
		filterType string
		value      string
		wantPaths  []string
	}{
		{"referrer_host", "source.test", wantHostPaths},
		{"referrer_host", " WWW.Source.Test ", wantHostPaths},
		{"referrer_host", "source.test.other.test", []string{"/suffix"}},
		{"referrer_host", "www-source.test", []string{"/hyphen"}},
		{"referrer", "source.test", []string{"/host", "/url"}},
		{"referrer", "www.source.test", []string{"/www-host"}},
		{"referrer", "WWW.source.test", []string{"/www-url"}},
	} {
		t.Run(tc.filterType+"="+tc.value, func(t *testing.T) {
			params := api.AnalyticsParams{
				SiteID: site.ID, Start: at.Add(-time.Hour), End: at.Add(time.Hour),
				Filters: []api.Filter{{Type: tc.filterType, Value: tc.value}},
			}
			stats, err := store.GetSiteStatsSections(t.Context(), params, SiteStatsSections{TopLists: true})
			if err != nil {
				t.Fatal(err)
			}
			gotPaths := make([]string, 0, len(stats.TopPages))
			for _, page := range stats.TopPages {
				gotPaths = append(gotPaths, page.Name)
			}
			slices.Sort(gotPaths)
			if diff := cmp.Diff(tc.wantPaths, gotPaths); diff != "" {
				t.Errorf("matched paths (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSelfReferrersExcludedFromReports(t *testing.T) {
	for _, domain := range []string{"example.test", "www.example.test"} {
		t.Run(domain, func(t *testing.T) {
			store, userID := setupEventBreakdownStore(t)
			ctx := t.Context()
			site, err := store.CreateSite(ctx, userID, domain)
			if err != nil {
				t.Fatal(err)
			}
			at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			refs := []string{"https://example.test/first?x=1", "https://www.example.test/next", "https://WWW.EXAMPLE.TEST:8443/next?x=1#part", "//www.example.test/next", "https://news.example.test/article", "https://external.test:8443/article?q=1", "https://example.test.evil.test/path", "https://www.www.example.test/path", "https://external.test/?next=https://example.test/", ""}
			for _, ref := range refs {
				session := uuid.New()
				hit := &api.Hit{SiteID: site.ID, SessionID: session, PageID: uuid.New(), Timestamp: at, Path: "/next", Referrer: new(ref)}
				if err := store.CreateHit(ctx, hit); err != nil {
					t.Fatal(err)
				}
				if err := store.CreateEvent(ctx, &api.Event{SiteID: site.ID, SessionID: session, Name: "signup", Timestamp: at}); err != nil {
					t.Fatal(err)
				}
			}
			params := api.AnalyticsParams{SiteID: site.ID, Start: at.Add(-time.Hour), End: at.Add(time.Hour)}
			want := map[string]int{"news.example.test": 1, "external.test:8443": 1, "example.test.evil.test": 1, "www.www.example.test": 1, "external.test": 1, "(Direct)": 1}
			check := func(got []api.MetricStat) {
				t.Helper()
				if len(got) != len(want) {
					t.Errorf("referrers = %+v, want %v", got, want)
				}
				for _, metric := range got {
					if want[metric.Name] != metric.Value {
						t.Errorf("unexpected referrer %+v", metric)
					}
				}
			}
			stats, err := store.GetSiteStats(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			check(stats.TopReferrers)
			if stats.TotalPageviews != len(refs) {
				t.Errorf("pageviews = %d", stats.TotalPageviews)
			}
			breakdown, err := store.GetDimensionBreakdown(ctx, params, "referrer", 20)
			if err != nil {
				t.Fatal(err)
			}
			metrics := []api.MetricStat{}
			for _, row := range breakdown {
				metrics = append(metrics, api.MetricStat{Name: row.Name, Value: row.Pageviews})
			}
			check(metrics)
			audience, err := store.GetEventAudience(ctx, api.EventAudienceParams{SiteID: site.ID, Start: params.Start, End: params.End, EventName: "signup"})
			if err != nil {
				t.Fatal(err)
			}
			check(audience.TopReferrers)
			for _, filterType := range []string{"referrer", "referrer_host"} {
				params.Filters = []api.Filter{{Type: filterType, Value: "www.example.test"}}
				filtered, err := store.GetSiteStats(ctx, params)
				if err != nil {
					t.Fatal(err)
				}
				if filtered.TotalPageviews != 0 {
					t.Errorf("self %s filter counted %d hits", filterType, filtered.TotalPageviews)
				}
			}
			other, err := store.CreateSite(ctx, userID, "other.test")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.CreateHit(ctx, &api.Hit{SiteID: other.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: at, Path: "/", Referrer: new("https://www.example.test/next")}); err != nil {
				t.Fatal(err)
			}
			params.SiteID, params.Filters = other.ID, nil
			isolated, err := store.GetSiteStats(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			if len(isolated.TopReferrers) != 1 || isolated.TopReferrers[0].Name != "www.example.test" {
				t.Errorf("other site's external referral lost: %+v", isolated.TopReferrers)
			}

			// Aggregate history has no per-hit hostname; use the configured site domain.
			sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			for _, ref := range []string{"example.test", "www.example.test", "news.example.test", "external.test:8443"} {
				if err := sink.PutDimension(ctx, importables.DimensionRow{Date: at, Dimension: "source", Name: ref, Pageviews: 3, SourceFile: "sources.csv"}); err != nil {
					t.Fatal(err)
				}
				if err := sink.PutEventDimension(ctx, importables.EventDimensionRow{Date: at, EventName: "signup", Dimension: "referrer", Name: ref, Visitors: 3, SourceFile: "events.csv"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := sink.Flush(ctx); err != nil {
				t.Fatal(err)
			}
			params.SiteID, params.Start, params.End = site.ID, at.AddDate(0, 0, -1), at.AddDate(0, 0, 1)
			history, err := store.GetSiteStats(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			want["news.example.test"], want["external.test:8443"] = 4, 4
			check(history.TopReferrers)
			audience, err = store.GetEventAudience(ctx, api.EventAudienceParams{SiteID: site.ID, Start: params.Start, End: params.End, EventName: "signup"})
			if err != nil {
				t.Fatal(err)
			}
			check(audience.TopReferrers)
			var raw string
			if err := store.db.QueryRowContext(ctx, "SELECT referrer FROM hits WHERE site_id = ? AND referrer = ? LIMIT 1", site.ID, refs[0]).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if raw != refs[0] {
				t.Fatal("stored historical referrer changed")
			}
		})
	}
}

func TestTenantSelfReferrerClassification(t *testing.T) {
	store := newTenantTestFixtureStore(t)
	ctx := t.Context()
	siteID := uuid.New()
	if _, err := store.db.ExecContext(ctx, "INSERT INTO sites (id,domain) VALUES (?,?)", siteID, "www.example.test"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ref  string
		want *string
	}{
		{"https://example.test/path?query=1", nil},
		{"https://www.example.test:8443/path", nil},
		{"example.test", nil},
		{"https://blog.example.test/path", new("blog.example.test")},
		{"https://external.test:8443/path?q=1", new("external.test:8443")},
		{"external.test/path?q=1", new("external.test/path?q=1")},
		{"https://example.test@external.test/path", new("example.test@external.test")},
		{"", new("(Direct)")},
	} {
		var got *string
		if err := store.db.QueryRowContext(ctx, "SELECT hk_external_referrer(?,?)", tc.ref, siteID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (tc.want == nil) || (got != nil && tc.want != nil && *got != *tc.want) {
			t.Errorf("referrer %q: got %v, want %v", tc.ref, got, tc.want)
		}
	}
}

func TestEcommerceSourcesSkipInternalNavigation(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := t.Context()
	site, err := store.CreateSite(ctx, userID, "example.test")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, ref := range []string{"https://www.example.test/cart", "https://external.test:8443/landing?q=1"} {
		session := uuid.New()
		for i, pageRef := range []string{ref, "https://example.test/checkout"} {
			if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: session, PageID: uuid.New(), Path: "/checkout", Timestamp: at.Add(time.Duration(i) * time.Minute), Referrer: new(pageRef)}); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.CreateEvent(ctx, &api.Event{SiteID: site.ID, SessionID: session, Name: "purchase", Timestamp: at.Add(2 * time.Minute), Properties: map[string]any{"transaction_id": session.String(), "value": 10, "currency": "EUR"}}); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := store.GetEcommerceSources(ctx, api.EcommerceParams{SiteID: site.ID, Start: at.Add(-time.Hour), End: at.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %+v", sources)
	}
	for _, source := range sources {
		if source.Referrer != "(Direct)" && source.Referrer != "external.test:8443" {
			t.Errorf("internal navigation attributed as source: %+v", source)
		}
		if source.Revenue != 10 {
			t.Errorf("revenue changed: %+v", source)
		}
	}
}
