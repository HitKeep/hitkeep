//go:build billing

package database

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type cloudRuntimeState struct {
	quotaGeneration atomic.Uint64
	welcomeOnce     sync.Once
	welcome         chan struct{}
}

func (s *Store) MonthlyQuotaGeneration() uint64 { return s.cloudState.quotaGeneration.Load() }

const activityCountWindow = 31 * 24 * time.Hour

func (s *Store) CurrentMonthlyHits(ctx context.Context, teamID uuid.UUID, now time.Time) (int64, error) {
	start := monthStart(now)
	query := `SELECT COALESCE(SUM(hits + events),0) FROM site_activity_hourly_counts WHERE bucket>=? AND bucket<?`
	args := []any{start, start.AddDate(0, 1, 0)}
	if s.tenantID == uuid.Nil {
		// Transfers preserve the activity buckets, including their original tenant_id.
		query += ` AND site_id IN (SELECT site_id FROM site_tenants WHERE tenant_id=?)`
		args = append(args, teamID)
	} else if s.tenantID != teamID {
		return 0, nil
	}
	var count int64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func monthStart(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}
