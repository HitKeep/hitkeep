package auth

import (
	"testing"
	"uuid"
)

func TestAuthorizeOAuthCeilings(t *testing.T) {
	siteID, teamID := uuid.NewV7(), uuid.NewV7()
	principal := Principal{Kind: PrincipalOAuth, SiteID: siteID, TeamID: teamID, SiteRole: SiteAdmin}
	resource := Resource{Kind: ResourceSite, ID: siteID, TeamID: teamID}
	newGrant := func() *OAuthGrant {
		return &OAuthGrant{ID: uuid.NewV7(), UserID: uuid.NewV7(), TeamID: teamID, SiteIDs: []uuid.UUID{siteID}, Scopes: []string{"sites:read", "analytics:read", "ingest:write"}, Audience: "https://example.test/api"}
	}
	for _, tc := range []struct {
		name   string
		change func(*OAuthGrant, *Principal, *Resource)
		action Capability
		scope  string
		want   bool
	}{
		{"read", func(*OAuthGrant, *Principal, *Resource) {}, Capability(PermSiteView), "analytics:read", true},
		{"ingest", func(*OAuthGrant, *Principal, *Resource) {}, Capability(PermSiteManageData), "ingest:write", true},
		{"wrong audience", func(g *OAuthGrant, _ *Principal, _ *Resource) { g.Audience = "https://example.test/mcp" }, Capability(PermSiteView), "analytics:read", false},
		{"missing scope", func(g *OAuthGrant, _ *Principal, _ *Resource) { g.Scopes = []string{"sites:read"} }, Capability(PermSiteView), "analytics:read", false},
		{"scope cannot grant action", func(*OAuthGrant, *Principal, *Resource) {}, Capability(PermSiteManageData), "analytics:read", false},
		{"unknown scope", func(g *OAuthGrant, _ *Principal, _ *Resource) { g.Scopes = []string{"admin"} }, Capability(PermSiteView), "admin", false},
		{"not consented", func(g *OAuthGrant, _ *Principal, _ *Resource) { g.SiteIDs = nil }, Capability(PermSiteView), "analytics:read", false},
		{"different team", func(g *OAuthGrant, _ *Principal, _ *Resource) { g.TeamID = uuid.NewV7() }, Capability(PermSiteView), "analytics:read", false},
		{"current role reduced", func(_ *OAuthGrant, p *Principal, _ *Resource) { p.SiteRole = SiteViewer }, Capability(PermSiteManageData), "ingest:write", false},
		{"instance cannot bypass", func(_ *OAuthGrant, p *Principal, _ *Resource) {
			p.SiteRole = SiteViewer
			p.InstanceRole = InstanceOwner
		}, Capability(PermSiteManageData), "ingest:write", false},
		{"unknown current role", func(_ *OAuthGrant, p *Principal, _ *Resource) { p.SiteRole = "unknown" }, Capability(PermSiteView), "analytics:read", false},
		{"missing user", func(g *OAuthGrant, _ *Principal, _ *Resource) { g.UserID = uuid.Nil() }, Capability(PermSiteView), "analytics:read", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, p, r := newGrant(), principal, resource
			tc.change(g, &p, &r)
			if got := AuthorizeOAuth(g, p, tc.action, r, "https://example.test/api", tc.scope); got != tc.want {
				t.Fatalf("AuthorizeOAuth() = %v, want %v", got, tc.want)
			}
		})
	}
}
