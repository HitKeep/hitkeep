package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"hitkeep/api"
)

var ErrAccountDeletionBlocked = errors.New("account deletion requires ownership handover")

// PlanAccountDeletion is advisory. Execution repeats it while membership and
// site ownership mutations are excluded; a preparation is never authorization.
func (s *Store) PlanAccountDeletion(ctx context.Context, userID uuid.UUID, choices api.AccountDeletionChoices, cloud bool) (api.AccountDeletionPlan, error) {
	plan := api.AccountDeletionPlan{Teams: []api.AccountDeletionTeam{}, Blockers: []api.AccountDeletionBlocker{}}
	u, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return plan, err
	}
	if u == nil {
		return plan, ErrUserNotFound
	}
	var lastOperator bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM instance_roles WHERE user_id=? AND role='owner') AND NOT EXISTS (SELECT 1 FROM instance_roles ir JOIN users u ON u.id=ir.user_id WHERE ir.user_id<>? AND ir.role='owner')`, userID, userID).Scan(&lastOperator); err != nil {
		return plan, err
	}
	if lastOperator {
		plan.Blockers = append(plan.Blockers, api.AccountDeletionBlocker{Code: "last_operator"})
	}
	successors := make(map[uuid.UUID]uuid.UUID)
	closing := make(map[uuid.UUID]bool)
	for _, selection := range choices.Successors {
		if selection.TeamID == uuid.Nil || selection.UserID == uuid.Nil || successors[selection.TeamID] != uuid.Nil {
			return plan, fmt.Errorf("invalid or duplicate successor")
		}
		successors[selection.TeamID] = selection.UserID
	}
	for _, id := range choices.CloseTeamIDs {
		if id == uuid.Nil || closing[id] || successors[id] != uuid.Nil {
			return plan, fmt.Errorf("invalid or conflicting team closure")
		}
		closing[id] = true
	}
	rows, err := s.db.QueryContext(ctx, `SELECT t.id,t.name,COALESCE(tm.role,'') FROM tenants t LEFT JOIN tenant_members tm ON tm.tenant_id=t.id AND tm.user_id=? WHERE tm.user_id IS NOT NULL OR EXISTS (SELECT 1 FROM sites s JOIN site_tenants st ON st.site_id=s.id WHERE st.tenant_id=t.id AND s.user_id=?) ORDER BY t.id`, userID, userID)
	if err != nil {
		return plan, err
	}
	type teamRole struct {
		id         uuid.UUID
		name, role string
	}
	var teams []teamRole
	for rows.Next() {
		var t teamRole
		if err := rows.Scan(&t.id, &t.name, &t.role); err != nil {
			rows.Close()
			return plan, err
		}
		teams = append(teams, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return plan, err
	}
	for _, t := range teams {
		team := api.AccountDeletionTeam{ID: t.id, Name: t.name, Action: "leave", Sites: []api.AccountDeletionSite{}, EligibleSuccessors: []api.AccountDeletionOwner{}, SuccessorID: successors[t.id]}
		members, err := s.ListTeamMembers(ctx, t.id)
		if err != nil {
			return plan, err
		}
		team.MemberCount = len(members)
		team.ClosureAllowed = cloud && t.role == TenantRoleOwner && len(members) == 1 && members[0].UserID == userID
		validSuccessor := false
		for _, m := range members {
			if m.UserID != userID && m.Role == TenantRoleOwner {
				team.EligibleSuccessors = append(team.EligibleSuccessors, api.AccountDeletionOwner{ID: m.UserID, Email: m.Email})
				if m.UserID == team.SuccessorID {
					validSuccessor = true
				}
			}
		}
		sites, err := s.accountDeletionSites(ctx, t.id, userID)
		if err != nil {
			return plan, err
		}
		ownsSites := false
		for _, site := range sites {
			owned := site.Owned
			ownsSites = ownsSites || owned
			team.Sites = append(team.Sites, api.AccountDeletionSite{ID: site.ID, Domain: site.Domain, Owned: owned})
		}
		switch {
		case closing[t.id]:
			if !cloud {
				plan.Blockers = append(plan.Blockers, api.AccountDeletionBlocker{Code: "closure_policy", TeamID: t.id})
			} else if t.role != TenantRoleOwner || len(members) != 1 || members[0].UserID != userID {
				plan.Blockers = append(plan.Blockers, api.AccountDeletionBlocker{Code: "ownership_transfer_required", TeamID: t.id})
			} else {
				team.Action = "close"
			}
		case t.role == TenantRoleOwner && len(team.EligibleSuccessors) == 0:
			plan.Blockers = append(plan.Blockers, api.AccountDeletionBlocker{Code: "ownership_transfer_required", TeamID: t.id})
		case ownsSites && !validSuccessor:
			plan.Blockers = append(plan.Blockers, api.AccountDeletionBlocker{Code: "successor_required", TeamID: t.id})
		case ownsSites:
			team.Action = "reparent"
		}
		if team.SuccessorID != uuid.Nil && !validSuccessor {
			plan.Blockers = append(plan.Blockers, api.AccountDeletionBlocker{Code: "invalid_successor", TeamID: t.id})
		}
		if cloud {
			team.BillingFingerprint, team.CancellationRequired, err = s.accountDeletionBilling(ctx, t.id)
			if err != nil {
				return plan, err
			}
		}
		delete(successors, t.id)
		delete(closing, t.id)
		plan.Teams = append(plan.Teams, team)
	}
	if len(successors) != 0 || len(closing) != 0 {
		return plan, fmt.Errorf("selection is not a current account team")
	}
	return plan, nil
}

// RemovePersonalAccount preserves shared resources. Solo team closure is owned
// by TenantStoreManager and must finish before this personal cleanup runs.
func (s *Store) RemovePersonalAccount(ctx context.Context, userID uuid.UUID, choices api.AccountDeletionChoices, cloud bool) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.primaryAuthMu.Lock()
	defer s.primaryAuthMu.Unlock()
	return s.removePersonalAccount(ctx, userID, choices, cloud)
}

func (s *Store) removePersonalAccount(ctx context.Context, userID uuid.UUID, choices api.AccountDeletionChoices, cloud bool) error {
	plan, err := s.PlanAccountDeletion(ctx, userID, choices, cloud)
	if err != nil {
		return err
	}
	if len(plan.Blockers) != 0 {
		return ErrAccountDeletionBlocked
	}
	for _, team := range plan.Teams {
		if team.Action == "close" {
			return ErrAccountDeletionBlocked
		}
		for _, site := range team.Sites {
			if site.Owned {
				if err := s.updateSiteIndexed(ctx, site.ID, "", team.SuccessorID); err != nil {
					return err
				}
			}
		}
	}
	user, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := runStoreTx(ctx, s.db, func(tx *sql.Tx) error { return cleanupUserRows(ctx, tx, userID) }); err != nil {
		return err
	}
	if user != nil {
		s.deletePasswordResetTokenForEmail(user.Email)
	}
	// DuckDB checks referenced parents against committed child changes.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, userID); err != nil {
		return fmt.Errorf("remove personal account parent: %w", err)
	}
	s.invalidateInstanceRole(userID)
	s.invalidateAllSiteRolesForUser(userID)
	return nil
}

func (s *Store) accountDeletionSites(ctx context.Context, teamID, userID uuid.UUID) ([]api.AccountDeletionSite, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.domain,s.user_id=? FROM sites s JOIN site_tenants st ON st.site_id=s.id WHERE st.tenant_id=? ORDER BY s.id`, userID, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sites := []api.AccountDeletionSite{}
	for rows.Next() {
		var site api.AccountDeletionSite
		if err := rows.Scan(&site.ID, &site.Domain, &site.Owned); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	return sites, rows.Err()
}
