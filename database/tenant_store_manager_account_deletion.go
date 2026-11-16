package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	json "hitkeep/jsonapi"
)

// ResumeAccountRemovals runs at startup before publishing stores. Only an
// intent past verified renewal cancellation can resume without another proof.
func (m *TenantStoreManager) ResumeAccountRemovals(ctx context.Context) error {
	rows, err := m.shared.db.QueryContext(ctx, `SELECT attempt_id,user_id,session_id,consequence_hash FROM account_deletion_intents WHERE phase='removing' ORDER BY created_at LIMIT 100`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var intents []AccountDeletionIntent
	for rows.Next() {
		var intent AccountDeletionIntent
		if err := rows.Scan(&intent.AttemptID, &intent.UserID, &intent.SessionID, &intent.ConsequenceHash); err != nil {
			rows.Close()
			return err
		}
		intents = append(intents, intent)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, intent := range intents {
		if err := m.DeleteCurrentAccount(ctx, intent.UserID, intent.AttemptID, intent.SessionID, intent.ConsequenceHash, nil, nil); err != nil {
			return fmt.Errorf("resume authorized account removal: %w", err)
		}
	}
	return nil
}

// DeleteCurrentAccount resumes only the exact, explicitly approved intent.
// authorize consumes auth-owned fresh proof; stopRenewals must verify provider
// state and retain its billing receipt before this owner can purge any data.
func (m *TenantStoreManager) DeleteCurrentAccount(ctx context.Context, userID uuid.UUID, attemptID, sessionID, expectedHash string, authorize func() bool, stopRenewals func(context.Context, api.AccountDeletionTeam, string) error) error {
	if m == nil || m.shared == nil || sessionID == "" {
		return ErrAccountDeletionUnavailable
	}
	s := m.shared
	s.billingLifecycleMu.Lock()
	defer s.billingLifecycleMu.Unlock()
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.primaryAuthMu.Lock()
	defer s.primaryAuthMu.Unlock()
	intent, err := s.GetAccountDeletionIntent(ctx, attemptID, userID)
	if err != nil {
		return err
	}
	if intent.ConsequenceHash != expectedHash {
		return ErrAccountDeletionChanged
	}
	if intent.Phase == "complete" {
		return nil
	}
	if intent.Phase == "prepared" || intent.Phase == "authorized" {
		if !time.Now().Before(intent.ExpiresAt) {
			return ErrAccountDeletionProof
		}
		fingerprint, err := s.AccountDeletionSecurityFingerprint(ctx, userID)
		if err != nil {
			return err
		}
		if intent.SecurityFingerprint == "" || fingerprint != intent.SecurityFingerprint {
			return ErrAccountDeletionProof
		}
		current, err := s.PlanAccountDeletion(ctx, userID, intent.Choices, intent.Cloud)
		if err != nil {
			return err
		}
		hash, err := AccountDeletionConsequenceHash(current)
		if err != nil {
			return err
		}
		if len(current.Blockers) != 0 || hash != expectedHash {
			return ErrAccountDeletionChanged
		}
		if intent.Phase == "prepared" {
			if authorize == nil || !authorize() {
				return ErrAccountDeletionProof
			}
			if err := s.setAccountDeletionPhase(ctx, attemptID, sessionID, "authorized"); err != nil {
				return err
			}
			intent.SessionID = sessionID
		}
		if intent.SessionID != sessionID {
			return ErrAccountDeletionProof
		}
		for _, team := range intent.Plan.Teams {
			if team.Action != "close" {
				continue
			}
			if !intent.Cloud || stopRenewals == nil {
				return ErrAccountDeletionUnavailable
			}
			if err := stopRenewals(ctx, team, attemptID); err != nil {
				return err
			}
			closed, err := s.IsClosedCloudAccount(ctx, team.ID, "", "")
			if err != nil {
				return err
			}
			if !closed {
				return ErrAccountDeletionUnavailable
			}
		}
		if err := s.setAccountDeletionPhase(ctx, attemptID, sessionID, "removing"); err != nil {
			return err
		}
		intent.Phase = "removing"
	}
	if intent.Phase != "removing" || intent.SessionID != sessionID {
		return ErrAccountDeletionProof
	}
	if !intent.ProvidersChecked {
		identities, err := s.ListUserSocialIdentities(ctx, userID)
		if err != nil {
			return err
		}
		appleLinked := false
		for _, identity := range identities {
			if identity.Provider == "apple" {
				appleLinked = true
			}
		}
		outcomes := []api.AccountDeletionProviderRevocation{}
		if appleLinked {
			if m.accountDeletionProviderRevoker == nil {
				return ErrAccountDeletionUnavailable
			}
			supplied := m.accountDeletionProviderRevoker(ctx, userID)
			outcome := api.AccountDeletionProviderRevocation{Provider: "apple", Status: "manual_revocation_required"}
			for _, value := range supplied {
				if value.Provider == "apple" && (value.Status == "revoked" || value.Status == "manual_revocation_required") {
					outcome = value
				}
			}
			outcomes = append(outcomes, outcome)
		}
		encoded, err := json.Marshal(outcomes)
		if err != nil {
			return err
		}
		if err := s.Exec(ctx, `UPDATE account_deletion_intents SET providers_checked=TRUE,provider_outcomes=?,updated_at=NOW() WHERE attempt_id=?`, string(encoded), attemptID); err != nil {
			return err
		}
		intent.ProvidersChecked = true
		intent.ProviderOutcomes = outcomes
	}
	for _, team := range intent.Plan.Teams {
		if team.Action == "close" {
			if err := m.purgeAccountDeletionTeam(ctx, intent, team.ID); err != nil {
				return err
			}
			continue
		}
		for _, site := range team.Sites {
			if !site.Owned {
				continue
			}
			current, err := s.GetSiteByID(ctx, site.ID)
			if err != nil {
				return err
			}
			// A coowner may delete the site after an earlier partial cleanup.
			if current == nil {
				continue
			}
			if current.UserID != userID && current.UserID != team.SuccessorID {
				return ErrAccountDeletionChanged
			}
			if current.UserID == userID {
				if err := s.updateSiteIndexed(ctx, site.ID, "", team.SuccessorID); err != nil {
					return err
				}
			}
			if err := m.SyncSite(ctx, site.ID); err != nil {
				return err
			}
		}
	}
	u, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if u != nil {
		if err := runStoreTx(ctx, s.db, func(tx *sql.Tx) error { return cleanupUserRows(ctx, tx, userID) }); err != nil {
			return err
		}
		s.deletePasswordResetTokenForEmail(u.Email)
		if _, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, userID); err != nil {
			return fmt.Errorf("remove account parent after committed cleanup: %w", err)
		}
	}
	s.invalidateInstanceRole(userID)
	s.invalidateAllSiteRolesForUser(userID)
	return s.setAccountDeletionPhase(ctx, attemptID, sessionID, "complete")
}

func (m *TenantStoreManager) purgeAccountDeletionTeam(ctx context.Context, intent *AccountDeletionIntent, teamID uuid.UUID) error {
	if !intent.Cloud || intent.Phase != "removing" {
		return ErrAccountDeletionBlocked
	}
	var exists bool
	if err := m.shared.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM tenants WHERE id=?)`, teamID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	var otherMembers int
	if err := m.shared.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenant_members WHERE tenant_id=? AND user_id<>?`, teamID, intent.UserID).Scan(&otherMembers); err != nil {
		return err
	}
	if otherMembers != 0 {
		return ErrAccountDeletionChanged
	}
	closed, err := m.shared.IsClosedCloudAccount(ctx, teamID, "", "")
	if err != nil {
		return err
	}
	if !closed {
		return ErrAccountDeletionUnavailable
	}
	if err := m.shared.Exec(ctx, `UPDATE closed_cloud_accounts SET removal_started=TRUE WHERE original_team_id=?`, teamID); err != nil {
		return err
	}
	sites, err := m.shared.accountDeletionSites(ctx, teamID, intent.UserID)
	if err != nil {
		return err
	}
	for _, site := range sites {
		if err := m.deleteSite(ctx, site.ID); err != nil {
			return err
		}
	}
	// The default catalog is the shared data-plane root. Its structural tenant
	// row/file must survive while other tenant catalogs remain attached.
	defaultID, err := m.DefaultTenantID(ctx)
	if err != nil {
		return err
	}
	if teamID != defaultID {
		if err := m.closeTenantStore(teamID); err != nil {
			return err
		}
		if err := os.RemoveAll(m.tenantDataDir(teamID)); err != nil {
			return fmt.Errorf("remove closed tenant data: %w", err)
		}
	}
	if err := m.shared.Transact(ctx, func(tx *sql.Tx) error {
		steps, err := buildScopedDeletePlan(ctx, tx, tenantPurgeSpec)
		if err != nil {
			return err
		}
		for _, step := range steps {
			if _, err := tx.ExecContext(ctx, step.query, teamID); err != nil {
				return fmt.Errorf("purge closed team %s: %w", step.table, err)
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE user_preferences SET active_tenant_id=NULL WHERE active_tenant_id=?`, teamID)
		return err
	}); err != nil {
		return err
	}
	if teamID == defaultID {
		if err := m.shared.Exec(ctx, `UPDATE tenants SET name=?,logo_url=NULL WHERE id=?`, defaultTenantName, teamID); err != nil {
			return err
		}
		return m.shared.Exec(ctx, `INSERT INTO tenant_archives (tenant_id,archived_at,archived_by) VALUES (?,NOW(),NULL) ON CONFLICT (tenant_id) DO NOTHING`, teamID)
	}
	return m.shared.Exec(ctx, `DELETE FROM tenants WHERE id=?`, teamID)
}
