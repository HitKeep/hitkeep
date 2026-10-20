package database

import (
	"testing"
	"time"
	"uuid"

	legacyuuid "github.com/google/uuid"

	"hitkeep/auth"
)

func TestAPIClientAuthReadsCurrentAuthority(t *testing.T) {
	for _, mutation := range []struct {
		name   string
		query  string
		denied bool
	}{
		{"grant removal", "DELETE FROM api_client_site_roles WHERE api_client_id = ?", false},
		{"external revoke", "UPDATE api_clients SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", true},
		{"external expiry", "UPDATE api_clients SET expires_at = TIMESTAMP '2000-01-01' WHERE id = ?", true},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			store, userID, siteID := setupAPIClientStore(t)
			defer store.Close()
			client, token, err := store.CreateAPIClient(t.Context(), userID, "external writer", "", auth.InstanceUser, map[legacyuuid.UUID]auth.SiteRole{siteID: auth.SiteViewer}, nil)
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.GetAPIClientAuth(t.Context(), token)
			if err != nil || before == nil {
				t.Fatalf("initial auth: %v, %v", before, err)
			}
			// An independent writer or older process cannot invalidate this Store's cache.
			if _, err := store.db.ExecContext(t.Context(), mutation.query, client.ID); err != nil {
				t.Fatal(err)
			}
			after, err := store.GetAPIClientAuth(t.Context(), token)
			if err != nil {
				t.Fatal(err)
			}
			if mutation.denied {
				if after != nil {
					t.Fatal("stale cached credential survived authority change")
				}
			} else if after == nil || len(after.SiteRoles) != 0 || after.ClientID != client.ID || after.UserID != userID {
				t.Fatalf("current grant/identity = %+v", after)
			}
		})
	}
}

func TestAPIClientAuthUsageHintDoesNotSlide(t *testing.T) {
	store, userID, siteID := setupAPIClientStore(t)
	defer store.Close()
	client, token, err := store.CreateAPIClient(t.Context(), userID, "usage", "", auth.InstanceUser, map[legacyuuid.UUID]auth.SiteRole{siteID: auth.SiteViewer}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAPIClientAuth(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	sentinel := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(t.Context(), "UPDATE api_clients SET last_used_at = ? WHERE id = ?", sentinel, client.ID); err != nil {
		t.Fatal(err)
	}
	if authz, err := store.GetAPIClientAuth(t.Context(), token); err != nil || authz == nil {
		t.Fatalf("repeat auth: %v, %v", authz, err)
	}
	var lastUsed time.Time
	if err := store.db.QueryRowContext(t.Context(), "SELECT last_used_at FROM api_clients WHERE id = ?", client.ID).Scan(&lastUsed); err != nil {
		t.Fatal(err)
	}
	if !lastUsed.Equal(sentinel) {
		t.Fatal("usage hint caused a write on every request")
	}
}

func TestGetSiteRoleForTenantIgnoresDashboardSelectionAndReloadsMembership(t *testing.T) {
	store, userID, siteID := setupAPIClientStore(t)
	defer store.Close()
	teamID, err := store.GetSiteTenantID(t.Context(), siteID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateTenant(t.Context(), userID, "other dashboard team", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveTenantID(t.Context(), userID, other.ID); err != nil {
		t.Fatal(err)
	}
	role, err := store.GetSiteRoleForTenant(t.Context(), uuid.UUID(userID), uuid.UUID(siteID), uuid.UUID(teamID))
	if err != nil || role != auth.SiteOwner {
		t.Fatalf("consent-team role: %s, %v", role, err)
	}
	if _, err := store.GetSiteRoleForTenant(t.Context(), uuid.UUID(userID), uuid.UUID(siteID), uuid.UUID(other.ID)); err == nil {
		t.Fatal("site accepted under another team")
	}
	if _, err := store.db.ExecContext(t.Context(), "DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?", teamID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSiteRoleForTenant(t.Context(), uuid.UUID(userID), uuid.UUID(siteID), uuid.UUID(teamID)); err == nil {
		t.Fatal("removed membership survived in cached or legacy site ownership")
	}
}

func TestPersonalAPIClientLosesCurrentActorRights(t *testing.T) {
	store, userID, siteID := setupAPIClientStore(t)
	defer store.Close()
	_, token, err := store.CreateAPIClient(t.Context(), userID, "current actor", "", auth.InstanceOwner, map[legacyuuid.UUID]auth.SiteRole{siteID: auth.SiteOwner}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAPIClientAuth(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), "UPDATE site_members SET role = 'viewer' WHERE site_id = ? AND user_id = ?", siteID, userID); err != nil {
		t.Fatal(err)
	}
	authz, err := store.GetAPIClientAuth(t.Context(), token)
	if err != nil || authz == nil {
		t.Fatalf("auth after demotion: %+v, %v", authz, err)
	}
	if authz.SiteRoles[siteID] != auth.SiteViewer {
		t.Fatalf("delegation escaped actor ceiling: %v", authz.SiteRoles)
	}
	teamID, err := store.GetSiteTenantID(t.Context(), siteID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), "DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?", teamID, userID); err != nil {
		t.Fatal(err)
	}
	authz, err = store.GetAPIClientAuth(t.Context(), token)
	if err != nil || authz == nil || len(authz.SiteRoles) != 0 {
		t.Fatalf("removed actor membership still authorizes: %+v, %v", authz, err)
	}
}
