package sites

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
	json "hitkeep/jsonapi"
)

func TestVisibleTimeReportRoute(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()
	ctx := t.Context()
	h.ctx.Config.JWTSecret = "test-visible-time-secret"
	h.ctx.Config.PublicURL = "https://hitkeep.example"
	site, err := store.CreateSite(ctx, userID, "visible.example")
	if err != nil {
		t.Fatal(err)
	}
	otherUserID, err := store.CreateUser(ctx, "other@example.com", "hashed")
	if err != nil {
		t.Fatal(err)
	}
	otherSite, err := store.CreateSite(ctx, otherUserID, "other-visible.example")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	Register(mux, h.ctx)
	path := "/api/sites/" + site.ID.String() + "/visible-time"
	get := func(target string, user uuid.UUID) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if user != uuid.Nil {
			token, err := auth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, user)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	for _, test := range []struct {
		name string
		url  string
		user uuid.UUID
		want int
	}{
		{"anonymous", path, uuid.Nil, http.StatusUnauthorized},
		{"other site member", path, otherUserID, http.StatusForbidden},
		{"share route", "/api/share/test/sites/" + site.ID.String() + "/visible-time", uuid.Nil, http.StatusNotFound},
		{"invalid date", path + "?from=bad", userID, http.StatusBadRequest},
		{"invalid filter", path + "?filter=bad:value", userID, http.StatusBadRequest},
		{"invalid goal", path + "?goal_id=bad", userID, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := get(test.url, test.user)
			if w.Code != test.want {
				t.Fatalf("got %d, want %d: %s", w.Code, test.want, w.Body)
			}
		})
	}

	t.Run("tracked views and session cohort", func(t *testing.T) {
		start := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
		end := start.Add(time.Hour)
		matched, unmatched := uuid.New(), uuid.New()
		addHit := func(siteID, sessionID uuid.UUID, path string, timestamp time.Time) uuid.UUID {
			t.Helper()
			pageID := uuid.New()
			if err := store.CreateHit(ctx, &api.Hit{SiteID: siteID, SessionID: sessionID, PageID: pageID, Path: path, Timestamp: timestamp}); err != nil {
				t.Fatal(err)
			}
			return pageID
		}
		addSample := func(siteID, pageID uuid.UUID, ms int) {
			t.Helper()
			if err := store.Exec(ctx, "INSERT INTO page_visible_time (site_id, page_id, visible_ms) VALUES (?, ?, ?)", siteID, pageID, ms); err != nil {
				t.Fatal(err)
			}
		}
		retriedPage := addHit(site.ID, matched, "/pricing", start)
		addSample(site.ID, retriedPage, 0)
		if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: matched, PageID: retriedPage, Path: "/pricing", Timestamp: start}); err != nil {
			t.Fatal(err)
		}
		addSample(site.ID, addHit(site.ID, matched, "/pricing", end), 5000)
		addSample(otherSite.ID, addHit(site.ID, unmatched, "/pricing", start), 20000)
		addSample(site.ID, addHit(site.ID, unmatched, "/pricing", end.Add(time.Second)), 9000)
		addSample(site.ID, addHit(site.ID, matched, "/zero", start), 0)
		addHit(site.ID, unmatched, "/without-sample", start)
		for range 2 {
			if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: unmatched, Path: "/without-sample", Timestamp: start}); err != nil {
				t.Fatal(err)
			}
		}
		addHit(otherSite.ID, unmatched, "/outside-site", start)
		addSample(site.ID, uuid.New(), 20000)
		addHit(site.ID, matched, "/signup", start)
		goal := &api.Goal{SiteID: site.ID, Name: "Signup", Type: "path", Value: "/signup"}
		if err := store.CreateGoal(ctx, goal); err != nil {
			t.Fatal(err)
		}

		q := url.Values{"from": {start.Format(time.RFC3339)}, "to": {end.Format(time.RFC3339)}}
		type reportRow struct {
			Path          string   `json:"path"`
			AvgVisibleMS  *float64 `json:"avg_visible_ms"`
			MeasuredViews int      `json:"measured_views"`
			TrackedViews  int      `json:"tracked_views"`
		}
		load := func() []reportRow {
			t.Helper()
			w := get(path+"?"+q.Encode(), userID)
			if w.Code != http.StatusOK {
				t.Fatalf("got %d: %s", w.Code, w.Body)
			}
			var rows []reportRow
			if err := json.UnmarshalRead(w.Body, &rows); err != nil {
				t.Fatal(err)
			}
			return rows
		}
		rows := load()
		if len(rows) != 4 {
			t.Fatalf("want four site paths, got %+v", rows)
		}
		pricing, missing, zero := rows[0], rows[1], rows[3]
		if pricing.Path != "/pricing" || pricing.TrackedViews != 3 || pricing.MeasuredViews != 2 || pricing.AvgVisibleMS == nil || *pricing.AvgVisibleMS != 2500 {
			t.Fatalf("unexpected pricing row: %+v", pricing)
		}
		if missing.Path != "/without-sample" || missing.TrackedViews != 3 || missing.MeasuredViews != 0 || missing.AvgVisibleMS != nil {
			t.Fatalf("unexpected missing row: %+v", missing)
		}
		if zero.Path != "/zero" || zero.MeasuredViews != 1 || zero.AvgVisibleMS == nil || *zero.AvgVisibleMS != 0 {
			t.Fatalf("unexpected zero row: %+v", zero)
		}

		q.Set("filter", "path:/pricing")
		q.Set("goal_id", goal.ID.String())
		rows = load()
		if len(rows) != 1 || rows[0].Path != "/pricing" || rows[0].TrackedViews != 2 || rows[0].MeasuredViews != 2 {
			t.Fatalf("unexpected filtered cohort: %+v", rows)
		}
		q.Set("filter", "path:/absent")
		if rows = load(); rows == nil || len(rows) != 0 {
			t.Fatalf("want empty report, got %#v", rows)
		}
	})
}
