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
	SecurityFingerprint                          string
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
	fingerprint, err := s.AccountDeletionSecurityFingerprint(ctx, intent.UserID)
	if err != nil {
		return err
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
	return s.Exec(ctx, `INSERT INTO account_deletion_intents (attempt_id,user_id,consequence_hash,choices,consequences,cloud,expires_at,security_fingerprint) VALUES (?,?,?,?,?,?,?,?)`, intent.AttemptID, intent.UserID, hash, string(choices), string(plan), intent.Cloud, intent.ExpiresAt, fingerprint)
}

func (s *Store) GetAccountDeletionIntent(ctx context.Context, attemptID string, userID uuid.UUID) (*AccountDeletionIntent, error) {
	intent := &AccountDeletionIntent{AttemptID: attemptID, UserID: userID}
	var choices, plan, outcomes string
	err := s.db.QueryRowContext(ctx, `SELECT session_id,consequence_hash,choices,consequences,cloud,phase,expires_at,providers_checked,provider_outcomes,security_fingerprint FROM account_deletion_intents WHERE attempt_id=? AND user_id=?`, attemptID, userID).Scan(&intent.SessionID, &intent.ConsequenceHash, &choices, &plan, &intent.Cloud, &intent.Phase, &intent.ExpiresAt, &intent.ProvidersChecked, &outcomes, &intent.SecurityFingerprint)
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
		return s.Exec(ctx, `UPDATE account_deletion_intents SET phase='complete',choices='',consequences='',security_fingerprint='',updated_at=NOW() WHERE user_id=(SELECT user_id FROM account_deletion_intents WHERE attempt_id=?)`, attemptID)
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

// AccountDeletionSecurityFingerprint excludes factor usage so a fresh MFA login
// preserves review, while credential changes also invalidate durable retries.
func (s *Store) AccountDeletionSecurityFingerprint(ctx context.Context, userID uuid.UUID) (string, error) {
	var material string
	err := s.db.QueryRowContext(ctx, `SELECT CAST(to_json(struct_pack(
		identity := struct_pack(email := u.email, password := u.password, password_enabled := COALESCE(u.password_login_enabled, TRUE)),
		totp := (SELECT list(secret ORDER BY secret) FROM user_totp_factors WHERE user_id=u.id),
		passkeys := (SELECT list(struct_pack(id := id, credential_id := credential_id, public_key := public_key) ORDER BY id) FROM user_passkeys WHERE user_id=u.id),
		social := (SELECT list(struct_pack(provider := provider, subject := subject, linked_at := linked_at) ORDER BY provider) FROM social_identities WHERE user_id=u.id),
		sso := (SELECT list(struct_pack(tenant_id := si.tenant_id, issuer := si.issuer_url, subject := si.subject, configured_issuer := cfg.issuer_url, client_id := cfg.client_id, enabled := cfg.enabled) ORDER BY si.tenant_id,si.issuer_url,si.subject) FROM sso_identities si LEFT JOIN team_sso_configs cfg ON cfg.tenant_id=si.tenant_id WHERE si.user_id=u.id),
		recovery := (SELECT list(code_hash ORDER BY code_hash) FROM user_recovery_codes WHERE user_id=u.id)
	)) AS VARCHAR) FROM users u WHERE u.id=?`, userID).Scan(&material)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(material))), nil
}
