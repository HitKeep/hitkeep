package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
	json "hitkeep/jsonapi"
)

var ErrAdminUserDeletionForbidden = errors.New("current instance owner authority required")

type AdminUserDeletionReviewError struct {
	Review api.AdminDeleteUserBlockedResponse
}

func (e *AdminUserDeletionReviewError) Error() string { return e.Review.Code }

type AdminUserDeletionResult struct {
	Deliveries    []WebhookDeliveryJob
	ArchivedTeams []uuid.UUID
}

func (s *Store) requireAnotherInstanceOwner(ctx context.Context, target uuid.UUID) error {
	var last bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM instance_roles WHERE user_id=? AND role='owner') AND NOT EXISTS (SELECT 1 FROM instance_roles r JOIN users u ON u.id=r.user_id WHERE r.user_id<>? AND r.role='owner' AND NOT EXISTS (SELECT 1 FROM account_deletion_intents d WHERE d.user_id=r.user_id AND d.phase IN ('removing','complete')))`, target, target).Scan(&last)
	if err != nil {
		return err
	}
	if last {
		return ErrLastInstanceOwner
	}
	return nil
}

func (s *Store) AdminDeleteUser(ctx context.Context, actor, target uuid.UUID, force bool, expected string, event WebhookEventInput) (AdminUserDeletionResult, error) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.adminDeleteUser(ctx, actor, target, force, expected, event, s.DeleteSiteWithWebhookEvent)
}

func (m *TenantStoreManager) AdminDeleteUser(ctx context.Context, actor, target uuid.UUID, force bool, expected string, event WebhookEventInput) (AdminUserDeletionResult, error) {
	m.shared.lifecycleMu.Lock()
	defer m.shared.lifecycleMu.Unlock()
	return m.shared.adminDeleteUser(ctx, actor, target, force, expected, event, m.deleteSiteWithWebhookEvent)
}

// Review comparison and execution share the lifecycle lock with membership and owner writes.
func (s *Store) adminDeleteUser(ctx context.Context, actor, target uuid.UUID, force bool, expected string, event WebhookEventInput, deleteSite func(context.Context, uuid.UUID, WebhookEventInput) ([]WebhookDeliveryJob, error)) (result AdminUserDeletionResult, err error) {
	var role string
	if err := s.db.QueryRowContext(ctx, `SELECT r.role FROM instance_roles r JOIN users u ON u.id=r.user_id WHERE r.user_id=?`, actor).Scan(&role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, ErrAdminUserDeletionForbidden
		}
		return result, err
	}
	if !auth.InstanceRole(role).HasPermission(auth.PermInstanceManageUsers) || actor == target {
		return result, ErrAdminUserDeletionForbidden
	}
	if err := s.requireAccountNotRemoving(ctx, actor); err != nil {
		return result, err
	}
	if err := s.requireAccountNotRemoving(ctx, target); err != nil {
		return result, err
	}
	user, err := s.GetUserByID(ctx, target)
	if err != nil {
		return result, err
	}
	if user == nil {
		return result, ErrUserNotFound
	}
	if err := s.requireAnotherInstanceOwner(ctx, target); err != nil {
		return result, err
	}
	teams, err := s.ListSoleOwnerTeams(ctx, target)
	if err != nil {
		return result, err
	}
	slices.SortFunc(teams, func(a, b api.Team) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	defaultID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return result, err
	}
	canForce := true
	sitesByID := map[uuid.UUID]api.AdminDeleteUserSite{}
	addSite := func(site api.Site) error {
		teamID, err := s.GetSiteTenantID(ctx, site.ID)
		if err != nil {
			return err
		}
		sitesByID[site.ID] = api.AdminDeleteUserSite{ID: site.ID, TeamID: teamID, UserID: site.UserID, Domain: site.Domain}
		return nil
	}
	for _, team := range teams {
		canForce = canForce && team.ID != defaultID
		var blockedMember bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM tenant_members m JOIN account_deletion_intents d ON d.user_id=m.user_id WHERE m.tenant_id=? AND d.phase IN ('removing','complete'))`, team.ID).Scan(&blockedMember); err != nil {
			return result, err
		}
		closedDefault, err := s.IsClosedCloudAccount(ctx, defaultID, "", "")
		if err != nil {
			return result, err
		}
		canForce = canForce && !blockedMember && !closedDefault
		sites, err := s.ListSitesForTenant(ctx, team.ID)
		if err != nil {
			return result, err
		}
		for _, site := range sites {
			if err := addSite(site); err != nil {
				return result, err
			}
		}
	}
	owned, err := s.ListUserSiteIDs(ctx, target)
	if err != nil {
		return result, err
	}
	for _, id := range owned {
		site, err := s.GetSiteByID(ctx, id)
		if err != nil {
			return result, err
		}
		if site == nil {
			return result, fmt.Errorf("owned site %s not found", id)
		}
		if err := addSite(*site); err != nil {
			return result, err
		}
	}
	sites := make([]api.AdminDeleteUserSite, 0, len(sitesByID))
	for _, site := range sitesByID {
		sites = append(sites, site)
	}
	slices.SortFunc(sites, func(a, b api.AdminDeleteUserSite) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	encoded, err := json.Marshal(struct {
		Actor  uuid.UUID                 `json:"actor"`
		Target uuid.UUID                 `json:"target"`
		Teams  []api.Team                `json:"teams"`
		Sites  []api.AdminDeleteUserSite `json:"sites"`
	}{actor, target, teams, sites})
	if err != nil {
		return result, err
	}
	review := api.AdminDeleteUserBlockedResponse{Status: "error", Code: "user_owns_teams", Message: "Review ownership and every affected site before confirming deletion.", Teams: teams, Sites: sites, CanForce: canForce, ConsequenceHash: fmt.Sprintf("%x", sha256.Sum256(encoded))}
	switch {
	case !force && len(teams) > 0:
		return result, &AdminUserDeletionReviewError{Review: review}
	case force && !canForce:
		review.Code = "delete_policy_blocked"
	case force && expected == "":
		review.Code = "delete_confirmation_required"
	case force && expected != review.ConsequenceHash:
		review.Code = "delete_consequences_changed"
	default:
		for _, site := range sites {
			event.Data = map[string]any{"site_id": site.ID.String(), "domain": site.Domain}
			jobs, err := deleteSite(ctx, site.ID, event)
			result.Deliveries = append(result.Deliveries, jobs...)
			if err != nil {
				return result, err
			}
		}
		for _, team := range teams {
			if err := s.adminArchiveTenant(ctx, team.ID, actor); err != nil {
				return result, err
			}
			result.ArchivedTeams = append(result.ArchivedTeams, team.ID)
		}
		return result, s.deleteUser(ctx, target)
	}
	return result, &AdminUserDeletionReviewError{Review: review}
}
