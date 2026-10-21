package shared

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"uuid"

	legacyuuid "github.com/google/uuid"

	"hitkeep/auth"
	"hitkeep/oauth"
)

const OAuthGrantKey contextKey = "oauth_grant"

var ErrOAuthAccessDenied = errors.New("oauth access denied")

// RequireIntegrationAuth accepts integration credentials without dashboard cookies.
func (c *Context) RequireIntegrationAuth(requiredScope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractAPIClientToken(r)
		if token == "" {
			c.writeOAuthAuthenticationError(w)
			return
		}
		if !oauth.IsOAuthToken(token) {
			if c.Store == nil {
				http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
				return
			}
			c.RequireAPIClientAuth(next)(w, r)
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] != token {
			c.writeOAuthAuthenticationError(w)
			return
		}
		if c.OAuthProvider == nil {
			c.writeOAuthAuthenticationError(w)
			return
		}
		audience := c.OAuthProvider.Resource("api")
		grant, err := c.OAuthProvider.Authenticate(r.Context(), token, audience)
		if errors.Is(err, oauth.ErrUnavailable) {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		if err != nil || grant == nil {
			c.writeOAuthAuthenticationError(w)
			return
		}
		if !slices.Contains(grant.Scopes, requiredScope) {
			w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="`+requiredScope+`"`)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), OAuthGrantKey, grant)
		ctx = context.WithValue(ctx, UserIDKey, legacyuuid.UUID(grant.UserID))
		next(w, r.WithContext(ctx))
	}
}

func (c *Context) writeOAuthAuthenticationError(w http.ResponseWriter) {
	challenge := `Bearer error="invalid_token"`
	if c.OAuthProvider != nil {
		challenge += `, resource_metadata="` + c.OAuthProvider.ResourceMetadataURL("api") + `"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// AuthorizeOAuthSite revalidates current consent-team membership and site roles.
func (c *Context) AuthorizeOAuthSite(ctx context.Context, grant *auth.OAuthGrant, siteID uuid.UUID, expectedAudience, requiredScope string, permission auth.Permission) error {
	if c.Store == nil || grant == nil || expectedAudience == "" || grant.Audience != expectedAudience || !slices.Contains(grant.Scopes, requiredScope) || !slices.Contains(grant.SiteIDs, siteID) {
		return ErrOAuthAccessDenied
	}
	role, err := c.Store.GetSiteRoleForTenant(ctx, grant.UserID, siteID, grant.TeamID)
	if err != nil {
		return ErrOAuthAccessDenied
	}
	resource := auth.Resource{Kind: auth.ResourceSite, ID: siteID, TeamID: grant.TeamID}
	principal := auth.Principal{Kind: auth.PrincipalOAuth, SiteID: siteID, TeamID: grant.TeamID, SiteRole: role}
	if !auth.AuthorizeOAuth(grant, principal, auth.Capability(permission), resource, expectedAudience, requiredScope) {
		return ErrOAuthAccessDenied
	}
	return nil
}
