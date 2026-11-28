//go:build billing

package ingest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sync/singleflight"

	"hitkeep/database"
	"hitkeep/entitlements"
)

// Refresh persisted usage every minute and reserve capacity for accepted requests between reads.
type monthlyQuota struct {
	once  sync.Once
	cache *lru.Cache[uuid.UUID, *quotaDecision]
	loads singleflight.Group
}
type quotaDecision struct {
	limited    bool
	remaining  atomic.Int64
	until      time.Time
	generation uint64
}

func (h *handler) dropMonthlyHit(ctx context.Context, siteID uuid.UUID) bool {
	if h.ctx.Config == nil || !h.ctx.Config.CloudHosted || h.ctx.Store == nil {
		return false
	}
	team, err := h.ctx.Store.GetSiteTenantID(ctx, siteID)
	if err != nil {
		return false
	}
	q := &h.monthlyQuota
	q.once.Do(func() { q.cache, _ = lru.New[uuid.UUID, *quotaDecision](8192) })
	now := time.Now().UTC()
	generation := h.ctx.Store.MonthlyQuotaGeneration()
	if cached, ok := q.cache.Get(team); ok && now.Before(cached.until) && cached.generation == generation {
		return cached.consume()
	}
	result, err, _ := q.loads.Do(team.String(), func() (any, error) {
		account, err := h.ctx.Store.GetCloudBillingAccount(ctx, team)
		if err != nil && !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			return false, err
		}
		plan, _ := entitlements.EffectiveCloudPlan(account)
		if account == nil {
			plan = h.ctx.Config.CloudPlanCode
			if plan == "" {
				plan = database.CloudPlanFree
			}
		}
		entry := &quotaDecision{generation: generation}
		if plan == database.CloudPlanFree {
			operator, err := h.ctx.Store.IsTeamOwnedByOperator(ctx, team)
			if err != nil {
				return false, err
			}
			if !operator {
				store := h.ctx.Store
				if h.ctx.TenantStores != nil {
					store, err = h.ctx.TenantStores.ForTenant(ctx, team)
					if err != nil {
						return false, err
					}
				}
				hits, err := store.CurrentMonthlyHits(ctx, team, now)
				if err != nil {
					return false, err
				}
				entry.limited = true
				entry.remaining.Store(database.CloudFreePlanMonthlyHitLimit*12/10 - hits)
			}
		}
		until := now.Add(time.Minute)
		if until.Month() != now.Month() {
			until = time.Date(until.Year(), until.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
		entry.until = until
		q.cache.Add(team, entry)
		return entry, nil
	})
	if err != nil {
		return false
	}
	entry := result.(*quotaDecision)
	return entry.generation == h.ctx.Store.MonthlyQuotaGeneration() && entry.consume()
}

func (q *quotaDecision) consume() bool { return q.limited && q.remaining.Add(-1) < 0 }
