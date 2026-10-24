package database

import (
	"errors"
	"fmt"
	"maps"
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

func TestPersonalAPIClientManySiteBindingsMatchCurrentRoles(t *testing.T) {
	store, userID, firstSiteID := setupAPIClientStore(t)
	defaultTeamID, err := store.GetDefaultTenantID(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	otherUserID, err := store.CreateUserWithoutDefaultTenant(t.Context(), "other-owner@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	secondTeam, err := store.CreateTenant(t.Context(), userID, "second grant team", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, teamID := range []legacyuuid.UUID{defaultTeamID, secondTeam.ID} {
		if err := store.AddTeamMember(t.Context(), teamID, otherUserID, TenantRoleOwner, userID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(t.Context(), "UPDATE tenant_members SET role = 'admin' WHERE tenant_id = ? AND user_id = ?", secondTeam.ID, userID); err != nil {
		t.Fatal(err)
	}
	roles := []auth.SiteRole{auth.SiteOwner, auth.SiteAdmin, auth.SiteEditor, auth.SiteViewer}
	siteIDs := make([]legacyuuid.UUID, 100)
	grants := make(map[legacyuuid.UUID]auth.SiteRole, len(siteIDs))
	for i := range siteIDs {
		teamID := defaultTeamID
		if i >= 50 {
			teamID = secondTeam.ID
		}
		creatorID := userID
		if i%2 != 0 {
			creatorID = otherUserID
		}
		if err := store.SetActiveTenantID(t.Context(), creatorID, teamID); err != nil {
			t.Fatal(err)
		}
		siteID := firstSiteID
		if i != 0 {
			site, err := store.CreateSite(t.Context(), creatorID, fmt.Sprintf("grant-%d.example.test", i))
			if err != nil {
				t.Fatal(err)
			}
			siteID = site.ID
		}
		siteIDs[i] = siteID
		grants[siteID] = roles[(i/4)%len(roles)]
		if err := store.AddSiteMember(t.Context(), siteID, userID, roles[i%len(roles)], userID); err != nil {
			t.Fatal(err)
		}
		if i%5 == 0 {
			if _, err := store.db.ExecContext(t.Context(), "DELETE FROM site_members WHERE site_id = ? AND user_id = ?", siteID, userID); err != nil {
				t.Fatal(err)
			}
		}
		if i < 50 && i%10 == 0 {
			if _, err := store.db.ExecContext(t.Context(), "DELETE FROM site_tenants WHERE site_id = ?", siteID); err != nil {
				t.Fatal(err)
			}
		}
	}
	client, token, err := store.CreateAPIClient(t.Context(), userID, "many current bindings", "", auth.InstanceOwner, grants, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCurrentRoles := func() *APIClientAuth {
		t.Helper()
		current, err := store.GetAPIClientAuth(t.Context(), token)
		if err != nil || current == nil {
			t.Fatalf("load current auth: %+v, %v", current, err)
		}
		want := make(map[legacyuuid.UUID]auth.SiteRole)
		for siteID, delegated := range grants {
			teamID, err := store.querySiteTenantID(t.Context(), siteID)
			if err != nil {
				t.Fatal(err)
			}
			role, err := store.GetSiteRoleForTenant(t.Context(), uuid.UUID(userID), uuid.UUID(siteID), uuid.UUID(teamID))
			if errors.Is(err, errSiteAccessDenied) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			want[siteID] = auth.MinSiteRole(role, delegated)
		}
		if !maps.Equal(current.SiteRoles, want) {
			t.Fatalf("batched roles differ from current scalar roles: got %v, want %v", current.SiteRoles, want)
		}
		return current
	}
	current := assertCurrentRoles()
	if len(current.SiteRoles) != 100 || current.SiteRoles[siteIDs[0]] != auth.SiteOwner || current.SiteRoles[siteIDs[1]] != auth.SiteAdmin || current.SiteRoles[siteIDs[3]] != auth.SiteViewer || current.SiteRoles[siteIDs[50]] != auth.SiteAdmin {
		t.Fatalf("mixed explicit, legacy, inherited or delegated roles changed: %v", current.SiteRoles)
	}
	// Independent writes must affect the next authentication despite a warm usage hint.
	if _, err := store.db.ExecContext(t.Context(), "UPDATE site_members SET role = 'viewer' WHERE user_id = ?", userID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), "UPDATE instance_roles SET role = 'user' WHERE user_id = ?", userID); err != nil {
		t.Fatal(err)
	}
	current = assertCurrentRoles()
	if current.InstanceRole != auth.InstanceUser || current.SiteRoles[siteIDs[1]] != auth.SiteViewer {
		t.Fatal("current actor demotion was ignored")
	}
	foreignTeamID := createTestTenant(t, store, "unconsented destination")
	if _, err := store.db.ExecContext(t.Context(), "UPDATE site_tenants SET tenant_id = ? WHERE site_id = ?", foreignTeamID, siteIDs[1]); err != nil {
		t.Fatal(err)
	}
	current = assertCurrentRoles()
	if _, ok := current.SiteRoles[siteIDs[1]]; ok {
		t.Fatal("transferred site retained old-team authority")
	}
	if _, err := store.db.ExecContext(t.Context(), "INSERT INTO tenant_archives (tenant_id, archived_at, archived_by) VALUES (?, ?, ?)", secondTeam.ID, time.Now().UTC(), userID); err != nil {
		t.Fatal(err)
	}
	current = assertCurrentRoles()
	if len(current.SiteRoles) != 49 {
		t.Fatalf("archived team's grants survived: %d grants", len(current.SiteRoles))
	}
	captured := current
	_, token, err = store.RotateAPIClient(t.Context(), userID, client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if old, err := store.RevalidateAPIClientAuth(t.Context(), captured); err != nil || old != nil {
		t.Fatalf("captured credential survived rotation: %+v, %v", old, err)
	}
	assertCurrentRoles()
	if _, err := store.db.ExecContext(t.Context(), "DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?", defaultTeamID, userID); err != nil {
		t.Fatal(err)
	}
	if current := assertCurrentRoles(); len(current.SiteRoles) != 0 {
		t.Fatal("removed team membership survived through explicit or legacy ownership")
	}
	if _, err := store.db.ExecContext(t.Context(), "UPDATE api_clients SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", client.ID); err != nil {
		t.Fatal(err)
	}
	if current, err := store.GetAPIClientAuth(t.Context(), token); err != nil || current != nil {
		t.Fatalf("revoked many-site credential survived: %+v, %v", current, err)
	}
}

func BenchmarkPersonalAPIClientAuth(b *testing.B) {
	for _, count := range []int{1, 100} {
		b.Run(fmt.Sprintf("sites=%d", count), func(b *testing.B) {
			store := NewStore(":memory:", WithThreads(1), WithCheckpointInterval(0))
			if err := store.Connect(); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = store.Close() })
			if err := store.Migrate(b.Context()); err != nil {
				b.Fatal(err)
			}
			userID, err := store.CreateUser(b.Context(), "benchmark@example.test", "hash")
			if err != nil {
				b.Fatal(err)
			}
			grants := make(map[legacyuuid.UUID]auth.SiteRole, count)
			for i := range count {
				site, err := store.CreateSite(b.Context(), userID, fmt.Sprintf("benchmark-%d.example.test", i))
				if err != nil {
					b.Fatal(err)
				}
				grants[site.ID] = auth.SiteViewer
			}
			_, token, err := store.CreateAPIClient(b.Context(), userID, "benchmark", "", auth.InstanceUser, grants, nil)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := store.GetAPIClientAuth(b.Context(), token); err != nil {
				b.Fatal(err)
			}
			for b.Loop() {
				current, err := store.GetAPIClientAuth(b.Context(), token)
				if err != nil || current == nil || len(current.SiteRoles) != count {
					b.Fatalf("auth: %+v, %v", current, err)
				}
			}
		})
	}
}
