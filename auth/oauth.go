package auth

import (
	"slices"
	"uuid"
)

// AuthorizeOAuth applies protocol and consent ceilings to the current resource decision.
func AuthorizeOAuth(grant *OAuthGrant, principal Principal, action Capability, resource Resource, audience, scope string) bool {
	if grant == nil || grant.ID == uuid.Nil() || grant.UserID == uuid.Nil() || grant.TeamID == uuid.Nil() || audience == "" || grant.Audience != audience || !slices.Contains(grant.Scopes, scope) {
		return false
	}
	if resource.Kind != ResourceSite || resource.TeamID != grant.TeamID || !slices.Contains(grant.SiteIDs, resource.ID) {
		return false
	}
	// A protocol scope cannot be used as a broader application permission.
	switch scope {
	case "sites:read", "analytics:read", "tracking_domains:read":
		if action != Capability(PermSiteView) {
			return false
		}
	case "ingest:write":
		if action != Capability(PermSiteManageData) {
			return false
		}
	default:
		return false
	}
	principal.Kind = PrincipalOAuth
	principal.InstanceRole = ""
	return Authorize(principal, action, resource)
}
