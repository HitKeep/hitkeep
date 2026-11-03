package auth

import (
	"testing"
	"uuid"
)

func TestOAuthActionScopesCannotEscalate(t *testing.T) {
	site, team := uuid.NewV7(), uuid.NewV7()
	for _, tc := range []struct {
		scope      string
		permission Permission
	}{
		{"goals:write", PermSiteManageGoals},
		{"annotations:write", PermSiteManageAnnotations},
		{"sites:write", PermSiteManageData},
		{"opportunities:write", PermSiteManageData},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			grant := &OAuthGrant{ID: uuid.NewV7(), UserID: uuid.NewV7(), TeamID: team, SiteIDs: []uuid.UUID{site}, Scopes: []string{tc.scope}, Audience: "https://example.test/api"}
			principal := Principal{Kind: PrincipalOAuth, SiteID: site, TeamID: team, SiteRole: SiteAdmin}
			resource := Resource{Kind: ResourceSite, ID: site, TeamID: team}
			if !AuthorizeOAuth(grant, principal, Capability(tc.permission), resource, grant.Audience, tc.scope) {
				t.Fatal("explicit scoped action rejected")
			}
			for _, forbidden := range []Permission{PermSiteManageTeam, PermSiteManageWebhooks, PermSiteDelete, PermInstanceManageUsers} {
				if AuthorizeOAuth(grant, principal, Capability(forbidden), resource, grant.Audience, tc.scope) {
					t.Fatalf("scope escalated to %s", forbidden)
				}
			}
			principal.SiteRole = SiteViewer
			principal.InstanceRole = InstanceOwner
			if AuthorizeOAuth(grant, principal, Capability(tc.permission), resource, grant.Audience, tc.scope) {
				t.Fatal("instance owner bypassed delegated current site role")
			}
		})
	}
}
