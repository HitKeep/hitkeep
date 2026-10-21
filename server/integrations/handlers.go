package integrations

import (
	"encoding/json/v2"
	"net/http"
	"time"
	"uuid"

	googleuuid "github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/database"
	"hitkeep/oauth"
	"hitkeep/server/shared"
)

type handler struct{ ctx *shared.Context }

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	wrap := func(scope string, next http.HandlerFunc) http.HandlerFunc {
		return ctx.Handler(shared.HandlerConfig{RateLimiter: ctx.ApiLimiter}, ctx.RequireIntegrationAuth(scope, next))
	}
	mux.HandleFunc("GET /api/integrations/sites", wrap(oauth.ScopeSitesRead, h.sites))
	mux.HandleFunc("GET /api/integrations/sites/{id}/tracking-domains", wrap(oauth.ScopeTrackingDomainsRead, h.trackingDomains))
	mux.HandleFunc("GET /api/integrations/sites/{id}/preview", wrap(oauth.ScopeAnalyticsRead, h.preview))
}
func response(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, value)
}
func (h *handler) grant(w http.ResponseWriter, r *http.Request) *auth.OAuthGrant {
	grant, ok := r.Context().Value(shared.OAuthGrantKey).(*auth.OAuthGrant)
	if !ok || grant == nil {
		response(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil
	}
	return grant
}
func (h *handler) site(w http.ResponseWriter, r *http.Request, scope string) (*auth.OAuthGrant, uuid.UUID, bool) {
	grant := h.grant(w, r)
	if grant == nil {
		return nil, uuid.Nil(), false
	}
	siteID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		response(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return nil, uuid.Nil(), false
	}
	if h.ctx.OAuthProvider == nil || !h.ctx.OAuthProvider.Ready() {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return nil, uuid.Nil(), false
	}
	if err := h.ctx.AuthorizeOAuthSite(r.Context(), grant, siteID, h.ctx.OAuthProvider.Resource("api"), scope, auth.PermSiteView); err != nil {
		response(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return nil, uuid.Nil(), false
	}
	return grant, siteID, true
}
func (h *handler) sites(w http.ResponseWriter, r *http.Request) {
	if h.ctx.OAuthProvider == nil || !h.ctx.OAuthProvider.Ready() {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	grant := h.grant(w, r)
	if grant == nil {
		return
	}
	sites, err := h.ctx.Store.ListSitesForTenant(r.Context(), googleuuid.UUID(grant.TeamID))
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	result := make([]api.IntegrationSite, 0, len(grant.SiteIDs))
	for _, site := range sites {
		siteID := uuid.UUID(site.ID)
		if err := h.ctx.AuthorizeOAuthSite(r.Context(), grant, siteID, h.ctx.OAuthProvider.Resource("api"), oauth.ScopeSitesRead, auth.PermSiteView); err != nil {
			continue
		}
		result = append(result, api.IntegrationSite{ID: siteID, Domain: site.Domain})
	}
	response(w, http.StatusOK, result)
}
func (h *handler) trackingDomains(w http.ResponseWriter, r *http.Request) {
	grant, _, ok := h.site(w, r, oauth.ScopeTrackingDomainsRead)
	if !ok {
		return
	}
	result := api.IntegrationTrackingDomains{DefaultScriptURL: h.ctx.OAuthProvider.Issuer() + "/hk.js", ActiveCustomDomains: []api.IntegrationTrackingDomain{}}
	if h.ctx.Limits().AllowsCustomTrackingDomains(r.Context(), googleuuid.UUID(grant.UserID), googleuuid.UUID(grant.TeamID)) {
		domains, err := h.ctx.Store.ListCustomTrackingDomains(r.Context(), googleuuid.UUID(grant.TeamID))
		if err != nil {
			response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
			return
		}
		for _, domain := range domains {
			if !database.CustomTrackingDomainIsActive(domain) {
				continue
			}
			result.ActiveCustomDomains = append(result.ActiveCustomDomains, api.IntegrationTrackingDomain{Hostname: domain.Hostname, ScriptURL: "https://" + domain.Hostname + "/hk.js"})
		}
	}
	response(w, http.StatusOK, result)
}
func (h *handler) preview(w http.ResponseWriter, r *http.Request) {
	grant, siteID, ok := h.site(w, r, oauth.ScopeAnalyticsRead)
	if !ok {
		return
	}
	if r.URL.RawQuery != "" {
		response(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	store, err := h.ctx.AnalyticsStore(r.Context(), googleuuid.UUID(siteID))
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	end := time.Now().UTC()
	start := end.Add(-7 * 24 * time.Hour)
	stats, err := store.GetSiteOverviewStats(r.Context(), api.AnalyticsParams{SiteID: googleuuid.UUID(siteID), UserID: googleuuid.UUID(grant.UserID), Start: start, End: end})
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	response(w, http.StatusOK, api.IntegrationPreview{SiteID: siteID, Start: start, End: end, TotalPageviews: stats.TotalPageviews, UniqueSessions: stats.UniqueSessions, BounceRate: stats.BounceRate})
}
