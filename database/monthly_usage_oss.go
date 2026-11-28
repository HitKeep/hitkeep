//go:build !billing

package database

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type cloudRuntimeState struct{}

func (*cloudRuntimeState) notifySignupVerified() {}

const activityCountWindow = 8 * 24 * time.Hour

func (s *Store) CurrentMonthlyHits(ctx context.Context, teamID uuid.UUID, now time.Time) (int64, error) {
	return 0, nil
}

func (s *Store) notifyCloudSignupVerified() { s.cloudState.notifySignupVerified() }
