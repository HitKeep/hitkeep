package shared

import (
	"context"
	"net/http"
	"strings"
	"uuid"

	legacyuuid "github.com/google/uuid"

	"hitkeep/auth"
	"hitkeep/oauth"
)

const embedClaimsKey contextKey = "embed_claims"

// EmbedAnalyticsRoute is deliberately closed: new dashboard routes need an explicit review.
func EmbedAnalyticsRoute(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	id := r.PathValue("id")
	if id == "" {
		return false
	}
	if r.URL.Path == "/api/embed/sites/"+id+"/site" {
		return true
	}
	suffix, ok := strings.CutPrefix(r.URL.Path, "/api/sites/"+id+"/")
	if !ok {
		return false
	}
	switch suffix {
	case "stats", "realtime", "goals", "goals/timeseries", "funnels", "funnels/timeseries", "events/names", "events/properties", "events/breakdown", "events/timeseries", "events/audience", "ecommerce", "ecommerce/timeseries", "ecommerce/products", "ecommerce/sources", "ai-activity", "web-vitals/summary", "web-vitals/timeseries", "web-vitals/pages", "web-vitals/breakdown":
		return true
	}
	parts := strings.Split(suffix, "/")
	if len(parts) == 3 && parts[0] == "funnels" && parts[2] == "stats" {
		_, err := uuid.Parse(parts[1])
		return err == nil
	}
	return false
}

// RequireEmbedAuth accepts only a site-bound dedicated embed bearer credential.
func (c *Context) RequireEmbedAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !EmbedAnalyticsRoute(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if c.Config == nil || c.Store == nil || c.OAuthProvider == nil || !c.OAuthProvider.Ready() {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.writeOAuthAuthenticationError(w)
			return
		}
		claims, err := auth.ValidateEmbedToken(parts[1], c.Config.JWTSecret, c.Config.PublicURL)
		if err != nil || claims.ParentOrigin != r.Header.Get("X-Hitkeep-Embed-Parent") || claims.SiteID.String() != r.PathValue("id") {
			c.writeOAuthAuthenticationError(w)
			return
		}
		stored, err := c.Store.GetOAuthGrant(r.Context(), claims.GrantID)
		if err != nil || stored == nil || !stored.Active {
			c.writeOAuthAuthenticationError(w)
			return
		}
		grant := &auth.OAuthGrant{ID: stored.ID, UserID: stored.UserID, TeamID: stored.TeamID, SiteIDs: stored.SiteIDs, Scopes: stored.Scopes, Audience: stored.Audience}
		if err := c.AuthorizeOAuthSite(r.Context(), grant, claims.SiteID, c.OAuthProvider.Resource("api"), oauth.ScopeAnalyticsRead, auth.PermSiteView); err != nil {
			c.writeOAuthAuthenticationError(w)
			return
		}
		ctx := context.WithValue(r.Context(), embedClaimsKey, claims)
		ctx = context.WithValue(ctx, OAuthGrantKey, grant)
		ctx = context.WithValue(ctx, UserIDKey, legacyuuid.UUID(grant.UserID))
		next(w, r.WithContext(ctx))
	}
}
