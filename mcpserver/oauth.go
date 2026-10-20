package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"uuid"

	legacyuuid "github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/analyticstools"
	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/oauth"
	"hitkeep/server/shared"
)

type oauthContextKey struct{}

func oauthGrantFromContext(ctx context.Context) *auth.OAuthGrant {
	grant, _ := ctx.Value(oauthContextKey{}).(*auth.OAuthGrant)
	return grant
}

func (s *service) oauthAudience() string {
	if s.oauthProvider == nil {
		return ""
	}
	return s.oauthProvider.Resource("mcp")
}

// authenticateOAuth returns true whenever this credential class handled the request.
func (s *service) authenticateOAuth(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	token := extractBearerToken(r)
	if !oauth.IsOAuthToken(token) {
		return false
	}
	if s.oauthProvider == nil {
		s.oauthError(w, http.StatusUnauthorized)
		return true
	}
	grant, err := s.oauthProvider.Authenticate(r.Context(), token, s.oauthAudience())
	if errors.Is(err, oauth.ErrUnavailable) {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return true
	}
	if err != nil || grant == nil {
		s.oauthError(w, http.StatusUnauthorized)
		return true
	}
	if !hasMCPScopes(grant) {
		s.oauthError(w, http.StatusForbidden)
		return true
	}
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), oauthContextKey{}, grant)))
	return true
}

func hasMCPScopes(grant *auth.OAuthGrant) bool {
	return grant != nil && slices.Contains(grant.Scopes, "sites:read") && slices.Contains(grant.Scopes, "analytics:read")
}

func (s *service) oauthError(w http.ResponseWriter, status int) {
	if status == http.StatusForbidden {
		w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="sites:read analytics:read"`)
		http.Error(w, "Forbidden", status)
		return
	}
	challenge := `Bearer error="invalid_token"`
	if s.oauthProvider != nil {
		challenge += `, resource_metadata="` + s.oauthProvider.ResourceMetadataURL("mcp") + `"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	http.Error(w, "Unauthorized", status)
}

func (s *service) oauthChallenge() string {
	challenge := "Bearer"
	if s.oauthProvider != nil {
		challenge += ` resource_metadata="` + s.oauthProvider.ResourceMetadataURL("mcp") + `"`
	}
	return challenge
}

func (s *service) authorizeOAuthSite(ctx context.Context, grant *auth.OAuthGrant, siteID uuid.UUID) error {
	if !hasMCPScopes(grant) {
		return errors.New("forbidden")
	}
	boundary := shared.Context{Store: s.store}
	return boundary.AuthorizeOAuthSite(ctx, grant, siteID, s.oauthAudience(), "analytics:read", auth.PermSiteView)
}

func (s *service) resolveOAuthSite(ctx context.Context, grant *auth.OAuthGrant, siteID legacyuuid.UUID) (analyticstools.Site, error) {
	if err := s.authorizeOAuthSite(ctx, grant, uuid.UUID(siteID)); err != nil {
		return analyticstools.Site{}, err
	}
	analytics := s.store
	if s.tenantStores != nil {
		var err error
		if analytics, _, err = s.tenantStores.ResolveSiteStore(ctx, siteID); err != nil {
			return analyticstools.Site{}, err
		}
	}
	return analyticstools.Site{ID: siteID, UserID: legacyuuid.UUID(grant.UserID), Control: s.store, Analytics: analytics}, nil
}

func (s *service) listOAuthSites(ctx context.Context, grant *auth.OAuthGrant) (*mcp.CallToolResult, listSitesOutput, error) {
	sites, err := s.store.ListSitesForTenant(ctx, legacyuuid.UUID(grant.TeamID))
	if err != nil {
		return nil, listSitesOutput{}, err
	}
	filtered := make([]api.Site, 0, len(sites))
	for _, site := range sites {
		if s.authorizeOAuthSite(ctx, grant, uuid.UUID(site.ID)) == nil {
			filtered = append(filtered, site)
		}
	}
	return nil, listSitesOutput{Sites: toMCPSites(filtered)}, nil
}
