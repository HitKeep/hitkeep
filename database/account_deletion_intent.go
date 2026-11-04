package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	json "hitkeep/jsonapi"
)

var (
	ErrAccountDeletionChanged     = errors.New("account deletion consequences changed")
	ErrAccountDeletionProof       = errors.New("fresh account deletion proof required")
	ErrAccountDeletionUnavailable = errors.New("account deletion temporarily unavailable")
)

type AccountDeletionIntent struct {
	AttemptID, SessionID, ConsequenceHash, Phase string
	UserID                                       uuid.UUID
	Choices                                      api.AccountDeletionChoices
	Plan                                         api.AccountDeletionPlan
	Cloud                                        bool
	ExpiresAt                                    time.Time
	ProvidersChecked                             bool
	ProviderOutcomes                             []api.AccountDeletionProviderRevocation
}

func AccountDeletionConsequenceHash(plan api.AccountDeletionPlan) (string, error) {
	encoded, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

func (s *Store) SaveAccountDeletionIntent(ctx context.Context, intent AccountDeletionIntent) error {
	if intent.AttemptID == "" || intent.UserID == uuid.Nil || len(intent.Plan.Blockers) != 0 || !intent.ExpiresAt.After(time.Now()) || intent.ExpiresAt.After(time.Now().Add(10*time.Minute)) {
		return ErrAccountDeletionBlocked
	}
	hash, err := AccountDeletionConsequenceHash(intent.Plan)
	if err != nil {
		return err
	}
	if hash != intent.ConsequenceHash {
		return ErrAccountDeletionChanged
	}
	choices, err := json.Marshal(intent.Choices)
	if err != nil {
		return err
	}
	plan, err := json.Marshal(intent.Plan)
	if err != nil {
		return err
	}
	if err := s.Exec(ctx, `DELETE FROM account_deletion_intents WHERE phase IN ('prepared','authorized') AND expires_at<=NOW()`); err != nil {
		return err
	}
	return s.Exec(ctx, `INSERT INTO account_deletion_intents (attempt_id,user_id,consequence_hash,choices,consequences,cloud,expires_at) VALUES (?,?,?,?,?,?,?)`, intent.AttemptID, intent.UserID, hash, string(choices), string(plan), intent.Cloud, intent.ExpiresAt)
}

func (s *Store) GetAccountDeletionIntent(ctx context.Context, attemptID string, userID uuid.UUID) (*AccountDeletionIntent, error) {
	intent := &AccountDeletionIntent{AttemptID: attemptID, UserID: userID}
	var choices, plan, outcomes string
	err := s.db.QueryRowContext(ctx, `SELECT session_id,consequence_hash,choices,consequences,cloud,phase,expires_at,providers_checked,provider_outcomes FROM account_deletion_intents WHERE attempt_id=? AND user_id=?`, attemptID, userID).Scan(&intent.SessionID, &intent.ConsequenceHash, &choices, &plan, &intent.Cloud, &intent.Phase, &intent.ExpiresAt, &intent.ProvidersChecked, &outcomes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAccountDeletionProof
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(outcomes), &intent.ProviderOutcomes); err != nil {
		return nil, err
	}
	if choices != "" {
		if err := json.Unmarshal([]byte(choices), &intent.Choices); err != nil {
			return nil, err
		}
	}
	if plan != "" {
		if err := json.Unmarshal([]byte(plan), &intent.Plan); err != nil {
			return nil, err
		}
	}
	return intent, nil
}

func (s *Store) AccountRemovalStarted(ctx context.Context, userID uuid.UUID) (bool, error) {
	var removing bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM account_deletion_intents WHERE user_id=? AND phase IN ('removing','complete'))`, userID).Scan(&removing)
	return removing, err
}

func (s *Store) setAccountDeletionPhase(ctx context.Context, attemptID, sessionID, phase string) error {
	if phase == "complete" {
		return s.Exec(ctx, `UPDATE account_deletion_intents SET phase='complete',choices='',consequences='',updated_at=NOW() WHERE user_id=(SELECT user_id FROM account_deletion_intents WHERE attempt_id=?)`, attemptID)
	}

	return s.Exec(ctx, `UPDATE account_deletion_intents SET session_id=?,phase=?,updated_at=NOW(),choices=CASE WHEN ?='complete' THEN '' ELSE choices END,consequences=CASE WHEN ?='complete' THEN '' ELSE consequences END WHERE attempt_id=?`, sessionID, phase, phase, phase, attemptID)
}

// ClosedCloudAccount is a billing retention receipt, never an account identity.
type ClosedCloudAccount struct {
	TeamID                     uuid.UUID
	CustomerID, SubscriptionID string
	CancelAtPeriodEnd          bool
	CurrentPeriodEnd           *time.Time
}

func (s *Store) RecordClosedCloudAccount(ctx context.Context, receipt ClosedCloudAccount) error {
	if receipt.TeamID == uuid.Nil || !receipt.CancelAtPeriodEnd {
		return ErrAccountDeletionBlocked
	}
	var different bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM closed_cloud_accounts WHERE original_team_id=? AND (stripe_customer_id<>? OR stripe_subscription_id<>?))`, receipt.TeamID, receipt.CustomerID, receipt.SubscriptionID).Scan(&different); err != nil {
		return err
	}
	if different {
		return ErrAccountDeletionChanged
	}
	return s.Exec(ctx, `INSERT INTO closed_cloud_accounts (original_team_id,stripe_customer_id,stripe_subscription_id,cancel_at_period_end,current_period_end) VALUES (?,?,?,?,?) ON CONFLICT (original_team_id) DO NOTHING`, receipt.TeamID, receipt.CustomerID, receipt.SubscriptionID, true, receipt.CurrentPeriodEnd)
}

func (s *Store) IsClosedCloudAccount(ctx context.Context, teamID uuid.UUID, customerID, subscriptionID string) (bool, error) {
	var closed bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM closed_cloud_accounts WHERE original_team_id=? OR (?<>'' AND stripe_customer_id=?) OR (?<>'' AND stripe_subscription_id=?))`, teamID, customerID, customerID, subscriptionID, subscriptionID).Scan(&closed)
	return closed, err
}

func (s *Store) IsRemovedCloudAccount(ctx context.Context, teamID uuid.UUID, customerID, subscriptionID string) (bool, error) {
	var closed bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM closed_cloud_accounts WHERE removal_started AND (original_team_id=? OR (?<>'' AND stripe_customer_id=?) OR (?<>'' AND stripe_subscription_id=?)))`, teamID, customerID, customerID, subscriptionID, subscriptionID).Scan(&closed)
	return closed, err
}

func (s *Store) requireAccountNotRemoving(ctx context.Context, userID uuid.UUID) error {
	started, err := s.AccountRemovalStarted(ctx, userID)
	if err != nil {
		return err
	}
	if started {
		return ErrAccountDeletionBlocked
	}
	return nil
}

func (s *Store) GetAccountDeletionResult(ctx context.Context, attemptID string, userID uuid.UUID) (*api.AccountDeletionResult, error) {
	intent, err := s.GetAccountDeletionIntent(ctx, attemptID, userID)
	if err != nil {
		return nil, err
	}
	if intent.Phase != "complete" || !intent.ProvidersChecked {
		return nil, ErrAccountDeletionUnavailable
	}
	outcomes := intent.ProviderOutcomes
	if outcomes == nil {
		outcomes = []api.AccountDeletionProviderRevocation{}
	}
	return &api.AccountDeletionResult{Status: "deleted", ProviderRevocations: outcomes}, nil
}
