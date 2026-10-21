package opportunities

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/auth"
)

func TestToolBridgeReloadsCapturedHumanAuthority(t *testing.T) {
	store, siteID, teamID, viewerID, _ := setupToolBridgeStore(t)
	bridge := NewToolBridge(ToolBridgeConfig{Shared: store, Analytics: store, TeamID: teamID, SiteID: siteID, ActorID: viewerID, ActorType: "user", EffectiveUserID: viewerID, EffectiveSiteRole: auth.SiteViewer, From: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)})
	if _, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview"); err != nil {
		t.Fatal(err)
	}
	if err := store.Exec(t.Context(), "DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?", teamID, viewerID); err != nil {
		t.Fatal(err)
	}
	if _, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview"); err == nil {
		t.Fatal("captured role survived membership removal")
	}
}

func TestToolBridgeReloadsCapturedAPICredential(t *testing.T) {
	for _, change := range []string{"revoke", "rotate"} {
		t.Run(change, func(t *testing.T) {
			store, siteID, teamID, _, _ := setupToolBridgeStore(t)
			client, token, err := store.CreateTeamAPIClient(t.Context(), teamID, "deferred tools", "", map[uuid.UUID]auth.SiteRole{siteID: auth.SiteViewer}, nil)
			if err != nil {
				t.Fatal(err)
			}
			principal, err := store.GetAPIClientAuth(t.Context(), token)
			if err != nil || principal == nil {
				t.Fatalf("auth: %+v, %v", principal, err)
			}
			bridge := NewToolBridge(ToolBridgeConfig{Shared: store, Analytics: store, TeamID: teamID, SiteID: siteID, ActorID: client.ID, ActorType: "api_client", APIClientAuth: principal, EffectiveSiteRole: auth.SiteViewer, From: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)})
			if _, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview"); err != nil {
				t.Fatal(err)
			}
			if change == "rotate" {
				if _, _, err := store.RotateTeamAPIClient(t.Context(), teamID, client.ID); err != nil {
					t.Fatal(err)
				}
			} else if err := store.Exec(t.Context(), "UPDATE api_clients SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", client.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview"); err == nil {
				t.Fatal("captured credential survived " + change)
			}
		})
	}
}
