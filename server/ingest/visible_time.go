package ingest

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
	stduuid "uuid"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/time/rate"

	"hitkeep/api"
	"hitkeep/blocking"
	queue "hitkeep/ingest"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

const (
	visibleTimePath        = "/ingest/visible-time"
	maxVisibleTimeBody     = 256
	visibleTimeSiteRate    = 200
	visibleTimeSiteBurst   = 400
	visibleTimeGlobalRate  = 2_000
	visibleTimeGlobalBurst = 1_000
)

type visibleTimeQuota struct {
	mu     sync.Mutex
	sites  *lru.LRU[uuid.UUID, *rate.Limiter]
	global *rate.Limiter
}

func newVisibleTimeQuota() *visibleTimeQuota {
	return &visibleTimeQuota{
		sites:  lru.NewLRU[uuid.UUID, *rate.Limiter](10_000, nil, 3*time.Minute),
		global: rate.NewLimiter(visibleTimeGlobalRate, visibleTimeGlobalBurst),
	}
}

func (q *visibleTimeQuota) allow(siteID uuid.UUID) bool {
	q.mu.Lock()
	limiter, ok := q.sites.Get(siteID)
	if !ok {
		limiter = rate.NewLimiter(visibleTimeSiteRate, visibleTimeSiteBurst)
		q.sites.Add(siteID, limiter)
	}
	q.mu.Unlock()
	return limiter.Allow() && q.global.Allow()
}

func parseVisibleTime(body []byte) (api.PageVisibleTimeSample, bool) {
	var payload struct {
		PageID    stduuid.UUID `json:"pid"`
		VisibleMS *int         `json:"ms"`
	}
	if err := json.UnmarshalStrict(body, &payload); err != nil || payload.PageID == stduuid.Nil() ||
		payload.VisibleMS == nil || *payload.VisibleMS < 0 || *payload.VisibleMS > api.MaxPageVisibleTimeMS {
		return api.PageVisibleTimeSample{}, false
	}
	return api.PageVisibleTimeSample{PageID: uuid.UUID(payload.PageID), VisibleMS: *payload.VisibleMS}, true
}

func (h *handler) handleVisibleTime(quota *visibleTimeQuota) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxVisibleTimeBody)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.recordRejection()
			if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
				http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, "Bad request body", http.StatusBadRequest)
			}
			return
		}
		sample, ok := parseVisibleTime(body)
		if !ok {
			h.recordRejection()
			http.Error(w, "Bad request body", http.StatusBadRequest)
			return
		}
		if !h.isLeader() {
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
			h.forwardToLeader(w, r, visibleTimePath)
			return
		}

		origin, err := url.Parse(r.Header.Get("Origin"))
		if err != nil || origin.Hostname() == "" || (origin.Scheme != "http" && origin.Scheme != "https") ||
			origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" {
			h.recordRejection()
			http.Error(w, "Valid Origin header is required", http.StatusBadRequest)
			return
		}

		site, err := h.ctx.Store.FindSiteByDomain(r.Context(), normalizeOriginHostname(origin.Hostname()))
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to find visible time site", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if site == nil {
			h.recordRejection()
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if !quota.allow(site.ID) {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		tenantID, err := h.ctx.Store.GetSiteTenantID(r.Context(), site.ID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve visible time site tenant", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !h.allowCustomTrackingHostForSite(r, site.ID) {
			h.recordRejection()
			w.WriteHeader(http.StatusAccepted)
			return
		}

		trustedProxies := h.ctx.Config.GetTrustedProxyNetworks()
		userIP := shared.GetRealIP(r, trustedProxies)
		if h.ctx.IPFilter != nil && h.ctx.IPFilter.EvaluateTraffic(site.ID, blocking.TrafficExclusionContext{
			IP:          userIP,
			CountryCode: shared.CountryCodeFromRequest(r, trustedProxies),
			UserAgent:   r.UserAgent(),
		}).Blocked {
			h.recordRejection()
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if h.ctx.SpamFilter != nil && h.ctx.SpamFilter.Evaluate(site.Domain, userIP, nil).Blocked {
			h.recordSpamDrop()
			w.WriteHeader(http.StatusAccepted)
			return
		}

		// Path and referrer exclusions are enforced by requiring the original
		// pageview to have passed ingestion before storing any sample.
		sample.SiteID = site.ID
		sample.TenantID = tenantID
		if err := h.publishJSON(queue.VisibleTimeTopic, sample); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to publish page visible time", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}
}
