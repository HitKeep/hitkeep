package auth

import (
	"strings"
	"testing"
	"uuid"
)

func TestAuthorizeResourceBindings(t *testing.T) {
	siteID, otherSiteID, teamID := uuid.New(), uuid.New(), uuid.New()
	resource := Resource{Kind: ResourceSite, ID: siteID, TeamID: teamID}
	principal := Principal{Kind: PrincipalSession, SiteID: siteID, TeamID: teamID, SiteRole: SiteViewer}
	tests := []struct {
		name      string
		principal Principal
		resource  Resource
		action    Capability
		want      bool
	}{
		{"viewer", principal, resource, Capability(PermSiteView), true},
		{"both parents absent", Principal{Kind: PrincipalSession, SiteID: siteID, SiteRole: SiteOwner}, Resource{Kind: ResourceSite, ID: siteID}, Capability(PermSiteView), false},
		{"OAuth cannot manage instance", Principal{Kind: PrincipalOAuth, InstanceRole: InstanceOwner}, Resource{Kind: ResourceInstance}, Capability(PermInstanceManageUsers), false},
		{"viewer cannot write", principal, resource, Capability(PermSiteManageData), false},
		{"wrong site", principal, Resource{Kind: ResourceSite, ID: otherSiteID, TeamID: teamID}, Capability(PermSiteView), false},
		{"wrong team", principal, Resource{Kind: ResourceSite, ID: siteID, TeamID: uuid.New()}, Capability(PermSiteView), false},
		{"missing parent", principal, Resource{Kind: ResourceSite, ID: siteID}, Capability(PermSiteView), false},
		{"missing site", principal, Resource{Kind: ResourceSite, TeamID: teamID}, Capability(PermSiteView), false},
		{"unknown action", principal, resource, "site.unknown", false},
		{"unknown resource", principal, Resource{Kind: "unknown", ID: siteID, TeamID: teamID}, Capability(PermSiteView), false},
		{"unknown principal", Principal{Kind: "unknown", SiteID: siteID, TeamID: teamID, SiteRole: SiteOwner}, resource, Capability(PermSiteView), false},
		{"unknown role", Principal{Kind: PrincipalSession, SiteID: siteID, TeamID: teamID, SiteRole: "unknown"}, resource, Capability(PermSiteView), false},
		{"instance inheritance", Principal{Kind: PrincipalSession, SiteID: siteID, TeamID: teamID, InstanceRole: InstanceAdmin}, resource, Capability(PermSiteView), true},
		{"instance admin cannot delete", Principal{Kind: PrincipalSession, SiteID: siteID, TeamID: teamID, InstanceRole: InstanceAdmin}, resource, Capability(PermSiteDelete), false},
		{"human team capability", Principal{Kind: PrincipalSession, TeamID: teamID, TeamRole: "admin"}, Resource{Kind: ResourceTeam, ID: teamID}, CapTeamManageMembers, true},
		{"team token is not human", Principal{Kind: PrincipalTeamAPIClient, TeamID: teamID, TeamRole: "owner"}, Resource{Kind: ResourceTeam, ID: teamID}, CapTeamManageMembers, false},
		{"oauth is not human", Principal{Kind: PrincipalOAuth, TeamID: teamID, TeamRole: "owner"}, Resource{Kind: ResourceTeam, ID: teamID}, CapTeamManageMembers, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Authorize(tt.principal, tt.action, tt.resource); got != tt.want {
				t.Fatalf("Authorize = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthorizeLegacyRoleParity(t *testing.T) {
	siteID, teamID := uuid.New(), uuid.New()
	for instanceRole, permissions := range instancePermissions {
		for _, action := range permissions {
			kind, id, parentID := ResourceInstance, uuid.Nil(), uuid.Nil()
			if strings.HasPrefix(string(action), "site.") {
				kind, id, parentID = ResourceSite, siteID, teamID
			}
			if !Authorize(Principal{Kind: PrincipalSession, SiteID: siteID, TeamID: teamID, InstanceRole: instanceRole}, Capability(action), Resource{Kind: kind, ID: id, TeamID: parentID}) {
				t.Fatalf("lost instance permission %s for %s", action, instanceRole)
			}
		}
	}
	for role, permissions := range sitePermissions {
		for _, action := range permissions {
			if !Authorize(Principal{Kind: PrincipalPersonalAPIClient, SiteID: siteID, TeamID: teamID, SiteRole: role}, Capability(action), Resource{Kind: ResourceSite, ID: siteID, TeamID: teamID}) {
				t.Fatalf("lost site permission %s for %s", action, role)
			}
		}
	}
}
