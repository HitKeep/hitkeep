package auth

import (
	"slices"
	"strings"
	"uuid"
)

// ResourcePolicyVersion identifies the interpretation of existing compact role rows.
// Current rows stay authoritative; no copied projection can restore revoked access.
const ResourcePolicyVersion = 1

type ResourceKind string

const (
	ResourceInstance ResourceKind = "instance"
	ResourceTeam     ResourceKind = "team"
	ResourceSite     ResourceKind = "site"
)

type Resource struct {
	Kind   ResourceKind
	ID     uuid.UUID
	TeamID uuid.UUID
}

type PrincipalKind string

const (
	PrincipalSession           PrincipalKind = "session"
	PrincipalPersonalAPIClient PrincipalKind = "personal_api_client"
	PrincipalTeamAPIClient     PrincipalKind = "team_api_client"
	PrincipalOAuth             PrincipalKind = "oauth"
)

// Principal carries current effective roles and their resolved resource identity.
// Boundaries intersect delegation, eligibility and OAuth consent before evaluation.
type Principal struct {
	Kind         PrincipalKind
	InstanceRole InstanceRole
	SiteRole     SiteRole
	SiteID       uuid.UUID
	TeamRole     string
	TeamID       uuid.UUID
}

// OAuthGrant is an active credential validated by the OAuth provider.
// Requests still intersect audience, scopes, explicit consent and current roles.
// No recoverable secret crosses this boundary.
type OAuthGrant struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	TeamID   uuid.UUID
	SiteIDs  []uuid.UUID
	Scopes   []string
	Audience string
}

// Authorize evaluates canonical role bundles for exactly one resolved resource.
// Strict site-only adapters omit InstanceRole to preserve their fallback ceiling.
func Authorize(principal Principal, action Capability, resource Resource) bool {
	switch principal.Kind {
	case PrincipalSession, PrincipalPersonalAPIClient, PrincipalTeamAPIClient, PrincipalOAuth:
	default:
		return false
	}
	switch resource.Kind {
	case ResourceInstance:
		return principal.Kind != PrincipalOAuth && resource.ID == uuid.Nil() && resource.TeamID == uuid.Nil() && strings.HasPrefix(string(action), "instance.") && roleAllows(ResourceInstance, string(principal.InstanceRole), action)
	case ResourceTeam:
		return principal.Kind == PrincipalSession && resource.ID != uuid.Nil() && resource.ID == principal.TeamID && resource.TeamID == uuid.Nil() && roleAllows(ResourceTeam, principal.TeamRole, action)
	case ResourceSite:
		if resource.ID == uuid.Nil() || resource.TeamID == uuid.Nil() || resource.ID != principal.SiteID || resource.TeamID != principal.TeamID || !strings.HasPrefix(string(action), "site.") {
			return false
		}
		return roleAllows(ResourceSite, string(principal.SiteRole), action) || roleAllows(ResourceInstance, string(principal.InstanceRole), action)
	default:
		return false
	}
}

// roleAllows is the sole role-bundle lookup, including legacy encoding adapters.
func roleAllows(kind ResourceKind, role string, action Capability) bool {
	switch kind {
	case ResourceInstance:
		return slices.Contains(instancePermissions[InstanceRole(role)], Permission(action))
	case ResourceSite:
		return slices.Contains(sitePermissions[SiteRole(role)], Permission(action))
	case ResourceTeam:
		return slices.Contains(TeamCapabilities(role), string(action))
	default:
		return false
	}
}
