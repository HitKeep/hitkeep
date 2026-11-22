package ingest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"hitkeep/api"
	"hitkeep/blocking"
	"hitkeep/config"
	"hitkeep/database"
	queue "hitkeep/ingest"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

func TestVisibleTimePayload(t *testing.T) {
	pid := "41440da4-8320-4476-83cf-3d8e3241a4a7"
	for _, tc := range []struct {
		name string
		body string
		ok   bool
		ms   int
	}{
		{name: "zero", body: `{"pid":"` + pid + `","ms":0}`, ok: true},
		{name: "cap", body: `{"pid":"` + pid + `","ms":86400000}`, ok: true, ms: 86400000},
		{name: "over cap", body: `{"pid":"` + pid + `","ms":86400001}`},
		{name: "negative", body: `{"pid":"` + pid + `","ms":-1}`},
		{name: "fraction", body: `{"pid":"` + pid + `","ms":1.1}`},
		{name: "decimal", body: `{"pid":"` + pid + `","ms":1.0}`},
		{name: "string", body: `{"pid":"` + pid + `","ms":"1"}`},
		{name: "null", body: `{"pid":"` + pid + `","ms":null}`},
		{name: "missing ms", body: `{"pid":"` + pid + `"}`},
		{name: "missing pid", body: `{"ms":0}`},
		{name: "malformed pid", body: `{"pid":"invalid","ms":0}`},
		{name: "nil pid", body: `{"pid":"00000000-0000-0000-0000-000000000000","ms":0}`},
		{name: "unknown", body: `{"pid":"` + pid + `","ms":0,"path":"/ignored"}`},
		{name: "duplicate", body: `{"pid":"` + pid + `","ms":0,"ms":1}`},
		{name: "trailing", body: `{"pid":"` + pid + `","ms":0}{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sample, ok := parseVisibleTime([]byte(tc.body))
			if ok != tc.ok || (ok && (sample.PageID.String() != pid || sample.VisibleMS != tc.ms)) {
				t.Fatalf("sample=%+v valid=%v, expected valid=%v ms=%d", sample, ok, tc.ok, tc.ms)
			}
		})
	}
}

func TestRegisteredVisibleTimeIngest(t *testing.T) {
	producer := &capturingProducer{}
	h, cleanup := setupIngestHandler(t, func(ctx *shared.Context) { ctx.Producer = producer })
	defer cleanup()
	mux := http.NewServeMux()
	Register(mux, h.ctx)

	site, err := h.ctx.Store.FindSiteByDomain(t.Context(), "example.com")
	if err != nil || site == nil {
		t.Fatalf("find fixture site: %v", err)
	}
	if _, err := h.ctx.Store.CreateSiteTrafficExclusion(t.Context(), site.ID, database.TrafficExclusionValues{
		Type: "user_agent", UserAgent: "excluded-browser",
	}, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	filter := blocking.NewIPFilter(h.ctx.Store, testServerIngestLogger())
	if err := filter.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	h.ctx.IPFilter = filter
	h.ctx.SpamFilter = mustNewTestSpamFilter(t, blocking.SpamFeedData{NetworkDenylist: []string{"192.0.2.9/32"}})

	otherOwner, err := h.ctx.Store.CreateUser(t.Context(), "other-visible-time-owner@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherTeam, err := h.ctx.Store.CreateTenant(t.Context(), otherOwner, "Other team", "")
	if err != nil {
		t.Fatal(err)
	}
	createActiveIngestTrackingDomainForTeam(t, h.ctx.Store, otherTeam.ID, "track.other.example")

	pageID := uuid.New()
	body := `{"pid":"` + pageID.String() + `","ms":0}`
	for _, tc := range []struct {
		name   string
		origin string
		mutate func(*http.Request)
		status int
	}{
		{name: "valid", origin: "https://example.com", status: http.StatusAccepted},
		{name: "unknown site", origin: "https://missing.example", status: http.StatusAccepted},
		{name: "missing origin", status: http.StatusBadRequest},
		{name: "origin path", origin: "https://example.com/path", status: http.StatusBadRequest},
		{name: "speculative", origin: "https://example.com", mutate: func(r *http.Request) { r.Header.Set("Sec-Purpose", "prefetch;prerender") }, status: http.StatusAccepted},
		{name: "excluded agent", origin: "https://example.com", mutate: func(r *http.Request) { r.Header.Set("User-Agent", "excluded-browser") }, status: http.StatusAccepted},
		{name: "cross-team custom host", origin: "https://example.com", mutate: func(r *http.Request) { r.Host = "track.other.example" }, status: http.StatusAccepted},
		{name: "spam IP", origin: "https://example.com", mutate: func(r *http.Request) { r.RemoteAddr = "192.0.2.9:12345" }, status: http.StatusAccepted},
		{name: "too large", origin: "https://example.com", mutate: func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(body + strings.Repeat(" ", maxVisibleTimeBody)))
		}, status: http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, visibleTimePath, strings.NewReader(body))
			req.RemoteAddr = "198.51.100.1:12345"
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Content-Type", "text/plain")
			if tc.mutate != nil {
				tc.mutate(req)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.origin != "" && rec.Header().Get("Access-Control-Allow-Origin") != tc.origin {
				t.Fatalf("missing response CORS header: %v", rec.Header())
			}
		})
	}

	if len(producer.messages) != 1 || producer.messages[0].topic != queue.VisibleTimeTopic {
		t.Fatalf("expected only the valid sample queued, got %v", producer.messages)
	}
	var sample api.PageVisibleTimeSample
	if err := json.Unmarshal(producer.messages[0].body, &sample); err != nil {
		t.Fatal(err)
	}
	if sample.SiteID != site.ID || sample.TenantID == uuid.Nil || sample.PageID != pageID || sample.VisibleMS != 0 {
		t.Fatalf("unexpected queued sample: %+v", sample)
	}

	req := httptest.NewRequest(http.MethodOptions, visibleTimePath, nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Fatalf("preflight: status=%d headers=%v", rec.Code, rec.Header())
	}
}

func TestVisibleTimeSiteQuota(t *testing.T) {
	q := newVisibleTimeQuota()
	first, second := uuid.New(), uuid.New()
	q.sites.Add(first, rate.NewLimiter(0, 1))
	if !q.allow(first) || q.allow(first) || !q.allow(second) {
		t.Fatal("exhausting the first site's quota must not exhaust the second site's quota")
	}
	q.global = rate.NewLimiter(0, 0)
	if q.allow(second) {
		t.Fatal("global quota must limit otherwise eligible sites")
	}
}

func TestVisibleTimeFollowerPreservesBodyAndCORS(t *testing.T) {
	origin := "https://example.com"
	body := `{"pid":"` + uuid.NewString() + `","ms":23}`
	leader := httptest.NewServer(newIngestCORS().Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil || string(raw) != body || r.URL.Path != visibleTimePath || r.Header.Get("Origin") != origin || r.Host != "tracker.example.com" {
			t.Errorf("incorrect forwarded request: path=%q host=%q origin=%q body=%q err=%v", r.URL.Path, r.Host, r.Header.Get("Origin"), raw, err)
		}
		w.WriteHeader(http.StatusAccepted)
	})))
	t.Cleanup(leader.Close)
	ctx := &shared.Context{
		Cluster: testClusterState{leaderAddr: leader.Listener.Addr().String()},
		Config:  &config.Config{HTTPAddr: leader.Listener.Addr().String()},
	}
	mux := http.NewServeMux()
	Register(mux, ctx)
	req := httptest.NewRequest(http.MethodPost, "https://tracker.example.com"+visibleTimePath, strings.NewReader(body))
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	origins := rec.Header().Values("Access-Control-Allow-Origin")
	if rec.Code != http.StatusAccepted || len(origins) != 1 || origins[0] != origin {
		t.Fatalf("follower response: status=%d origins=%v body=%s", rec.Code, origins, rec.Body.String())
	}
}

func BenchmarkParseVisibleTime(b *testing.B) {
	body := []byte(`{"pid":"41440da4-8320-4476-83cf-3d8e3241a4a7","ms":48623}`)
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		if _, ok := parseVisibleTime(body); !ok {
			b.Fatal("fixture failed validation")
		}
	}
}
