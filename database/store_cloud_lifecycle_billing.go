//go:build billing

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	CloudLifecycleMessageZeroHitDay2           = "cloud_zero_hit_day2"
	CloudLifecycleMessageZeroHitDay6           = "cloud_zero_hit_day6"
	CloudLifecycleMessageWelcome               = "cloud_welcome"
	CloudLifecycleMessageFreeRetentionReminder = "cloud_free_retention_reminder"
	CloudLifecycleMessageFreeRetentionPreTrim  = "cloud_free_retention_pretrim"
	CloudLifecycleMessageFreeLimitReminder     = "cloud_free_limit_reminder"
	CloudLifecycleMessageStatusSent            = "sent"
	CloudLifecycleMessageStatusFailed          = "failed"
	CloudLifecycleMessageMaxAttempts           = 3

	// CloudFreePlanRetentionDays mirrors the free plan's MaxRetentionDays
	// entitlement so lifecycle eligibility can be computed in SQL.
	CloudFreePlanRetentionDays = 60
	// CloudRetentionPreTrimLeadDays is how many days before the first data
	// roll-off the pre-trim warning goes out.
	CloudRetentionPreTrimLeadDays = 7
)

var ErrCloudLifecycleMessageNotFound = errors.New("cloud lifecycle message not found")

type CloudLifecycleRecipient struct {
	MonthlyLimitNear   bool
	TenantID           uuid.UUID
	TenantName         string
	UserID             uuid.UUID
	Email              string
	Locale             string
	SiteID             uuid.UUID
	SiteDomain         string
	FirstHitAt         time.Time
	PlanCode           string
	PlanName           string
	SubscriptionStatus string
	Attempts           int
}

type CloudLifecycleMessage struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	UserID          uuid.UUID
	Kind            string
	Status          string
	Attempts        int
	ProcessingError string
	SentAt          *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CloudLifecycleMessageUpdate struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	Kind     string
	Error    string
	Now      time.Time
}

func (s *Store) ListEligibleCloudLifecycleRecipients(ctx context.Context, kind string, now time.Time, limit int) ([]CloudLifecycleRecipient, error) {
	return (&TenantStoreManager{shared: s}).ListEligibleCloudLifecycleRecipients(ctx, kind, now, limit)
}

func (s *Store) MarkCloudLifecycleMessageSent(ctx context.Context, update CloudLifecycleMessageUpdate) error {
	now := cloudLifecycleNow(update.Now)
	return s.Exec(ctx, `
		INSERT INTO cloud_lifecycle_messages (
			id,
			tenant_id,
			user_id,
			kind,
			status,
			attempts,
			processing_error,
			sent_at,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, 1, NULL, ?, ?, ?)
		ON CONFLICT (tenant_id, user_id, kind) DO UPDATE SET
			status = excluded.status,
			attempts = cloud_lifecycle_messages.attempts + 1,
			processing_error = NULL,
			sent_at = excluded.sent_at,
			updated_at = excluded.updated_at
	`, uuid.New(), update.TenantID, update.UserID, strings.TrimSpace(update.Kind), CloudLifecycleMessageStatusSent, now, now, now)
}

func (s *Store) MarkCloudLifecycleMessageFailed(ctx context.Context, update CloudLifecycleMessageUpdate) error {
	now := cloudLifecycleNow(update.Now)
	return s.Exec(ctx, `
		INSERT INTO cloud_lifecycle_messages (
			id,
			tenant_id,
			user_id,
			kind,
			status,
			attempts,
			processing_error,
			sent_at,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, 1, NULLIF(?, ''), NULL, ?, ?)
		ON CONFLICT (tenant_id, user_id, kind) DO UPDATE SET
			status = excluded.status,
			attempts = cloud_lifecycle_messages.attempts + 1,
			processing_error = excluded.processing_error,
			updated_at = excluded.updated_at
	`, uuid.New(), update.TenantID, update.UserID, strings.TrimSpace(update.Kind), CloudLifecycleMessageStatusFailed, truncateCloudLifecycleError(update.Error), now, now)
}

func (s *Store) GetCloudLifecycleMessage(ctx context.Context, tenantID, userID uuid.UUID, kind string) (*CloudLifecycleMessage, error) {
	var message CloudLifecycleMessage
	var processingError sql.NullString
	var sentAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, user_id, kind, status, attempts, COALESCE(processing_error, ''), sent_at, created_at, updated_at
		FROM cloud_lifecycle_messages
		WHERE tenant_id = ? AND user_id = ? AND kind = ?
	`, tenantID, userID, strings.TrimSpace(kind)).Scan(
		&message.ID,
		&message.TenantID,
		&message.UserID,
		&message.Kind,
		&message.Status,
		&message.Attempts,
		&processingError,
		&sentAt,
		&message.CreatedAt,
		&message.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCloudLifecycleMessageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query cloud lifecycle message: %w", err)
	}
	message.ProcessingError = strings.TrimSpace(processingError.String)
	if sentAt.Valid {
		sent := sentAt.Time
		message.SentAt = &sent
	}
	return &message, nil
}

func cloudLifecycleNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func truncateCloudLifecycleError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		return value[:1000]
	}
	return value
}

// CloudSignupVerified wakes the lifecycle worker after a verified signup. The durable
// billing account and message record also allow delivery to resume after restart.
func (s *Store) cloudSignupNotifications() chan struct{} {
	s.cloudState.welcomeOnce.Do(func() { s.cloudState.welcome = make(chan struct{}, 1) })
	return s.cloudState.welcome
}

func (s *Store) CloudSignupNotifications() <-chan struct{} { return s.cloudSignupNotifications() }
func (s *Store) notifyCloudSignupVerified() {
	select {
	case s.cloudSignupNotifications() <- struct{}{}:
	default:
	}
}
