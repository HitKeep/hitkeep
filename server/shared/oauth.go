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
const oauthActionScopeKey contextKey = "oauth_action_scope"

var ErrOAuthAccessDenied = errors.New("oauth access denied")

// RequireActionOAuth opts a route into delegated actions without changing its session/API-client access.
func (c *Context) RequireActionOAuth(scope string, next, ordinary http.HandlerFunc) http.HandlerFunc {
	delegated := c.RequireIntegrationAuth(scope, func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), oauthActionScopeKey, scope)
		next(w, r.WithContext(ctx))
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if !oauth.IsOAuthToken(extractAPIClientToken(r)) {
			ordinary(w, r)
			return
		}
		if c.Config == nil || !c.Config.AIEnabled {
			http.NotFound(w, r)
			return
		}
		delegated(w, r)
	}
}

// ResolveRequestTeamID keeps delegated requests on the team approved in consent.
func (c *Context) ResolveRequestTeamID(r *http.Request) (legacyuuid.UUID, error) {
	if grant, _ := r.Context().Value(OAuthGrantKey).(*auth.OAuthGrant); grant != nil {
		if grant.TeamID == uuid.Nil() || grant.UserID != uuid.UUID(GetUserIDFromContext(r)) {
			return legacyuuid.Nil, ErrOAuthAccessDenied
		}
		return legacyuuid.UUID(grant.TeamID), nil
	}
	return c.Store.GetActiveTenantID(r.Context(), GetUserIDFromContext(r))
}

// AuthorizeOAuthTeam permits only the explicit billing delegation, with current team authority.
func (c *Context) AuthorizeOAuthTeam(ctx context.Context, grant *auth.OAuthGrant, teamID uuid.UUID, scope string, capability auth.Capability) error {
	if c.Store == nil || c.OAuthProvider == nil || grant == nil || grant.ID == uuid.Nil() || grant.UserID == uuid.Nil() || teamID == uuid.Nil() || grant.TeamID != teamID || grant.Audience != c.OAuthProvider.Resource("api") || scope != oauth.ScopeBillingWrite || !slices.Contains(grant.Scopes, scope) || capability != auth.CapTeamManageSettings {
		return ErrOAuthAccessDenied
	}
	role, err := c.Store.GetTenantRole(ctx, legacyuuid.UUID(teamID), legacyuuid.UUID(grant.UserID))
	if err != nil || !auth.TeamRoleHasCapability(role, capability) {
		return ErrOAuthAccessDenied
	}
	return nil
}

func (c *Context) requireOAuthSitePermission(perm auth.Permission, next http.HandlerFunc, w http.ResponseWriter, r *http.Request, grant *auth.OAuthGrant) {
	siteID, err := siteIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	scope, _ := r.Context().Value(oauthActionScopeKey).(string)
	if c.OAuthProvider == nil {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}
	role, err := c.authorizeOAuthSite(r.Context(), grant, uuid.UUID(siteID), c.OAuthProvider.Resource("api"), scope, perm)
	if err != nil {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}
	ctx := context.WithValue(r.Context(), PermissionKey, PermissionContext{UserID: legacyuuid.UUID(grant.UserID), InstanceRole: auth.InstanceUser, SiteRole: role})
	next(w, r.WithContext(ctx))
}

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
	_, err := c.authorizeOAuthSite(ctx, grant, siteID, expectedAudience, requiredScope, permission)
	return err
}

func (c *Context) authorizeOAuthSite(ctx context.Context, grant *auth.OAuthGrant, siteID uuid.UUID, expectedAudience, requiredScope string, permission auth.Permission) (auth.SiteRole, error) {
	if c.Store == nil || grant == nil || expectedAudience == "" || grant.Audience != expectedAudience || !slices.Contains(grant.Scopes, requiredScope) || !slices.Contains(grant.SiteIDs, siteID) {
		return "", ErrOAuthAccessDenied
	}
	role, err := c.Store.GetSiteRoleForTenant(ctx, grant.UserID, siteID, grant.TeamID)
	if err != nil {
		return "", ErrOAuthAccessDenied
	}
	resource := auth.Resource{Kind: auth.ResourceSite, ID: siteID, TeamID: grant.TeamID}
	principal := auth.Principal{Kind: auth.PrincipalOAuth, SiteID: siteID, TeamID: grant.TeamID, SiteRole: role}
	if !auth.AuthorizeOAuth(grant, principal, auth.Capability(permission), resource, expectedAudience, requiredScope) {
		return "", ErrOAuthAccessDenied
	}
	return role, nil
}
