package oauth

import (
	"context"
	"encoding/json/v2"
	"errors"
	"mime"
	"net/http"
	"uuid"

	googleuuid "github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/appurl"
	"hitkeep/auth"
	"hitkeep/database"
	provider "hitkeep/oauth"
	"hitkeep/server/shared"
)

type handler struct{ ctx *shared.Context }

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	human := func(next func(http.ResponseWriter, *http.Request, uuid.UUID)) http.HandlerFunc {
		return ctx.Handler(shared.HandlerConfig{RequireAuth: true, HumanOnly: true, RateLimiter: ctx.ApiLimiter}, func(w http.ResponseWriter, r *http.Request) {
			userID, ok := h.humanUser(w, r)
			if ok {
				next(w, r, userID)
			}
		})
	}
	mux.HandleFunc("GET /api/oauth/continuation", human(h.continuation))
	mux.HandleFunc("GET /api/oauth/consent", human(h.consent))
	mux.HandleFunc("POST /api/oauth/consent", human(h.completeConsent))
	mux.HandleFunc("GET /api/user/oauth-grants", human(h.grants))
	mux.HandleFunc("DELETE /api/user/oauth-grants/{id}", human(h.revoke))
	h.registerClients(mux)
	p := ctx.OAuthProvider
	if p == nil {
		return
	}
	public := shared.HandlerConfig{RateLimiter: ctx.AuthLimiter}
	mux.HandleFunc("GET /oauth/authorize", ctx.Handler(public, p.Authorize))
	mux.HandleFunc("POST /oauth/token", ctx.Handler(public, p.Token))
	mux.HandleFunc("POST /oauth/revoke", ctx.Handler(public, p.Revoke))
	mux.HandleFunc("OPTIONS /oauth/token", ctx.Handler(public, p.Preflight))
	mux.HandleFunc("OPTIONS /oauth/revoke", ctx.Handler(public, p.Preflight))
	paths := p.DiscoveryPaths()
	mux.HandleFunc("GET "+paths[0], p.Metadata)
	mux.HandleFunc("GET "+paths[1], p.ProtectedResourceMetadata("api"))
	if len(paths) > 2 {
		mux.HandleFunc("GET "+paths[2], p.ProtectedResourceMetadata("mcp"))
	}
}
func response(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, value)
}
func (h *handler) humanUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	if _, ok := r.Context().Value(shared.AuthSessionKey).(shared.AuthSessionContext); !ok {
		response(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return uuid.Nil(), false
	}
	oldUserID := shared.GetUserIDFromContext(r)
	id := uuid.UUID(oldUserID)
	if id == uuid.Nil() {
		response(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return uuid.Nil(), false
	}
	user, err := h.ctx.Store.GetUserByID(r.Context(), oldUserID)
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return uuid.Nil(), false
	}
	if user == nil {
		response(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return uuid.Nil(), false
	}
	return id, true
}
func (h *handler) continuation(w http.ResponseWriter, r *http.Request, _ uuid.UUID) {
	p := h.ctx.OAuthProvider
	pending, err := p.HasContinuation(r.Context(), r)
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	response(w, http.StatusOK, map[string]bool{"pending": pending})
}
func (h *handler) authorizeSite(ctx context.Context, g *auth.OAuthGrant, siteID uuid.UUID) error {
	p := h.ctx.OAuthProvider
	if p == nil || !p.Ready() {
		return provider.ErrUnavailable
	}
	audience := ""
	if g.Audience == p.Resource("api") {
		audience = p.Resource("api")
	} else if p.Resource("mcp") != "" && g.Audience == p.Resource("mcp") {
		audience = p.Resource("mcp")
	}
	if audience == "" {
		return provider.ErrConsentForbidden
	}
	for _, scope := range g.Scopes {
		permission := auth.PermSiteView
		if scope == provider.ScopeIngestWrite {
			permission = auth.PermSiteManageData
		}
		if err := h.ctx.AuthorizeOAuthSite(ctx, g, siteID, audience, scope, permission); err != nil {
			return provider.ErrConsentForbidden
		}
	}
	return nil
}
func (h *handler) consent(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	p := h.ctx.OAuthProvider
	if p == nil || !p.Ready() {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	info, err := p.Consent(r.Context(), r, userID)
	if err != nil {
		status := http.StatusGone
		code := "expired"
		if errors.Is(err, provider.ErrUnavailable) {
			status = http.StatusServiceUnavailable
			code = "unavailable"
		}
		response(w, status, map[string]string{"error": code})
		return
	}
	teams, _, err := h.ctx.Store.ListUserTeams(r.Context(), googleuuid.UUID(userID))
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	for _, team := range teams {
		if info.RegistrationTeamID != uuid.Nil() && info.RegistrationTeamID != uuid.UUID(team.ID) {
			continue
		}
		sites, err := h.ctx.Store.ListSitesForTenant(r.Context(), team.ID)
		if err != nil {
			response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
			return
		}
		display := api.OAuthTeam{ID: uuid.UUID(team.ID), Name: team.Name, Sites: []api.OAuthSite{}}
		for _, site := range sites {
			candidate := &auth.OAuthGrant{ID: uuid.NewV7(), UserID: userID, TeamID: display.ID, SiteIDs: []uuid.UUID{uuid.UUID(site.ID)}, Scopes: info.Scopes, Audience: info.Resource}
			if h.authorizeSite(r.Context(), candidate, uuid.UUID(site.ID)) != nil {
				continue
			}
			display.Sites = append(display.Sites, api.OAuthSite{ID: uuid.UUID(site.ID), Domain: site.Domain})
		}
		if len(display.Sites) > 0 {
			info.Teams = append(info.Teams, display)
		}
	}
	response(w, http.StatusOK, info)
}
func (h *handler) completeConsent(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	p := h.ctx.OAuthProvider
	if p == nil {
		target := "/oauth/consent?error=unavailable"
		if h.ctx.Config != nil {
			target = appurl.Path(h.ctx.Config.PublicURL, "/oauth/consent") + "?error=unavailable"
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	fail := func(err error) { p.RedirectConsentError(w, r, err) }
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" || r.URL.RawQuery != "" {
		fail(provider.ErrInvalidConsent)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		fail(provider.ErrInvalidConsent)
		return
	}
	for _, key := range []string{"decision", "team_id", "csrf_token"} {
		if len(r.PostForm[key]) > 1 {
			fail(provider.ErrInvalidConsent)
			return
		}
	}
	var teamID uuid.UUID
	if r.PostForm.Get("decision") == "approve" {
		teamID, err = uuid.Parse(r.PostForm.Get("team_id"))
		if err != nil {
			fail(provider.ErrInvalidConsent)
			return
		}
	}
	rawSites := r.PostForm["site_ids"]
	if len(rawSites) > 512 {
		fail(provider.ErrInvalidConsent)
		return
	}
	sites := make([]uuid.UUID, 0, len(rawSites))
	for _, raw := range rawSites {
		id, err := uuid.Parse(raw)
		if err != nil {
			fail(provider.ErrInvalidConsent)
			return
		}
		sites = append(sites, id)
	}
	if err := p.CompleteConsent(w, r, userID, teamID, sites, r.PostForm.Get("decision"), r.PostForm.Get("csrf_token"), h.authorizeSite); err != nil {
		fail(err)
	}
}
func (h *handler) grants(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	if h.ctx.Cluster != nil && !h.ctx.Cluster.IsLeader() {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	grants, err := h.ctx.Store.ListOAuthGrants(r.Context(), userID)
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	teams, _, err := h.ctx.Store.ListUserTeams(r.Context(), googleuuid.UUID(userID))
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	names := make(map[uuid.UUID]string, len(teams))
	siteCache := make(map[uuid.UUID]map[uuid.UUID]string)
	for _, team := range teams {
		names[uuid.UUID(team.ID)] = team.Name
	}
	result := make([]api.OAuthAuthorizedApp, 0, len(grants))
	for _, grant := range grants {
		item := api.OAuthAuthorizedApp{ID: grant.ID, Client: api.OAuthClientSummary{ID: grant.ClientID, Name: grant.ClientName}, Team: api.OAuthTeamSummary{ID: grant.TeamID, Name: names[grant.TeamID]}, Sites: []api.OAuthSite{}, Scopes: grant.Scopes, Resource: grant.Audience, CreatedAt: grant.CreatedAt, ExpiresAt: grant.ExpiresAt, LastUsedAt: grant.LastUsedAt}
		if _, member := names[grant.TeamID]; member {
			if _, ok := siteCache[grant.TeamID]; !ok {
				sites, err := h.ctx.Store.ListSitesForTenant(r.Context(), googleuuid.UUID(grant.TeamID))
				if err != nil {
					response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
					return
				}
				siteCache[grant.TeamID] = make(map[uuid.UUID]string, len(sites))
				for _, site := range sites {
					siteCache[grant.TeamID][uuid.UUID(site.ID)] = site.Domain
				}
			}
			principal := &auth.OAuthGrant{ID: grant.ID, UserID: userID, TeamID: grant.TeamID, SiteIDs: grant.SiteIDs, Scopes: grant.Scopes, Audience: grant.Audience}
			for _, id := range grant.SiteIDs {
				if domain, ok := siteCache[grant.TeamID][id]; ok && h.authorizeSite(r.Context(), principal, id) == nil {
					item.Sites = append(item.Sites, api.OAuthSite{ID: id, Domain: domain})
				}
			}
		}
		result = append(result, item)
	}
	response(w, http.StatusOK, result)
}
func (h *handler) revoke(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	if h.ctx.Cluster != nil && !h.ctx.Cluster.IsLeader() {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		response(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if err := h.ctx.Store.RevokeOAuthGrant(r.Context(), userID, id); err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, database.ErrOAuthNotFound) {
			status = http.StatusNotFound
		}
		response(w, status, map[string]string{"error": "not_found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
