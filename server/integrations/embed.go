package integrations

import (
	"encoding/json/v2"
	"net/http"
	"net/url"
	"time"

	googleuuid "github.com/google/uuid"

	"hitkeep/auth"
	"hitkeep/oauth"
)

type embedResponse struct {
	EmbedURL     string    `json:"embed_url"`
	EmbedToken   string    `json:"embed_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	ParentOrigin string    `json:"parent_origin"`
	SiteID       string    `json:"site_id"`
}

func (h *handler) embed(w http.ResponseWriter, r *http.Request) {
	grant, siteID, ok := h.site(w, r, oauth.ScopeAnalyticsRead)
	if !ok {
		return
	}
	var input struct {
		ParentOrigin string `json:"parent_origin"`
	}
	if r.URL.RawQuery != "" || json.UnmarshalRead(http.MaxBytesReader(w, r.Body, 4096), &input, json.RejectUnknownMembers(true)) != nil || !auth.ValidEmbedParentOrigin(input.ParentOrigin) {
		response(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if h.ctx.Config == nil || h.ctx.Config.JWTSecret == "" {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	expires := time.Now().UTC().Add(auth.EmbedTokenLifetime).Truncate(time.Second)
	token, err := auth.GenerateEmbedToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, grant.ID, siteID, input.ParentOrigin, expires)
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	launch, err := auth.GenerateEmbedLaunch(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, siteID, input.ParentOrigin, expires)
	if err != nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	response(w, http.StatusCreated, embedResponse{EmbedURL: h.ctx.OAuthProvider.Issuer() + "/embed/" + siteID.String() + "?launch=" + url.QueryEscape(launch), EmbedToken: token, ExpiresAt: expires, ParentOrigin: input.ParentOrigin, SiteID: siteID.String()})
}

func (h *handler) embedSite(w http.ResponseWriter, r *http.Request) {
	// Authentication already binds this route to one current, read-only OAuth grant.
	siteID, err := googleuuid.Parse(r.PathValue("id"))
	if err != nil {
		response(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	site, err := h.ctx.Store.GetSiteByID(r.Context(), siteID)
	if err != nil || site == nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	response(w, http.StatusOK, map[string]any{"id": site.ID, "domain": site.Domain, "created_at": site.CreatedAt, "data_retention_days": site.DataRetentionDays, "retention_synced_from_plan": site.RetentionSyncedFromPlan, "role": auth.SiteViewer})
}
