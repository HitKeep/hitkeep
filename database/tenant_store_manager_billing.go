//go:build billing

package database

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

type lifecycleCandidate struct {
	CloudLifecycleRecipient
	created        time.Time
	sites, members int
	activated      bool
}

// Billing and delivery records live on the control plane. Activity belongs to the tenant.
func (m *TenantStoreManager) ListEligibleCloudLifecycleRecipients(ctx context.Context, kind string, now time.Time, limit int) ([]CloudLifecycleRecipient, error) {
	if m == nil || m.shared == nil {
		return nil, fmt.Errorf("tenant store manager is not configured")
	}
	now = cloudLifecycleNow(now)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	switch kind {
	case CloudLifecycleMessageWelcome, CloudLifecycleMessageZeroHitDay2, CloudLifecycleMessageZeroHitDay6, CloudLifecycleMessageFreeRetentionReminder, CloudLifecycleMessageFreeRetentionPreTrim, CloudLifecycleMessageFreeLimitReminder:
	default:
		return nil, fmt.Errorf("unsupported cloud lifecycle message kind %q", kind)
	}
	rows, err := m.shared.db.QueryContext(ctx, `
 SELECT t.id,t.name,t.created_at,tm.user_id,u.email,COALESCE(up.default_locale,''),
 cba.plan_code,cba.plan_name,cba.subscription_status,COALESCE(clm.attempts,0),
 (SELECT COUNT(*) FROM site_tenants WHERE tenant_id=t.id),
 (SELECT COUNT(*) FROM tenant_members WHERE tenant_id=t.id),
 EXISTS(SELECT 1 FROM cloud_conversion_events WHERE tenant_id=t.id AND event_name=?)
 FROM tenants t JOIN cloud_billing_accounts cba ON cba.tenant_id=t.id
 JOIN tenant_members tm ON tm.tenant_id=t.id AND tm.role='owner'
 JOIN users u ON u.id=tm.user_id
 LEFT JOIN user_preferences up ON up.user_id=u.id
 LEFT JOIN cloud_lifecycle_messages clm ON clm.tenant_id=t.id AND clm.user_id=u.id AND clm.kind=?
 WHERE NOT EXISTS(SELECT 1 FROM tenant_archives WHERE tenant_id=t.id)
 AND clm.sent_at IS NULL AND COALESCE(clm.status,'')<>? AND COALESCE(clm.attempts,0)<?
 `, CloudConversionFirstHitReceived, kind, CloudLifecycleMessageStatusSent, CloudLifecycleMessageMaxAttempts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []lifecycleCandidate
	for rows.Next() {
		var c lifecycleCandidate
		if err := rows.Scan(&c.TenantID, &c.TenantName, &c.created, &c.UserID, &c.Email, &c.Locale, &c.PlanCode, &c.PlanName, &c.SubscriptionStatus, &c.Attempts, &c.sites, &c.members, &c.activated); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var result []CloudLifecycleRecipient
	for _, c := range candidates {
		rescue := kind == CloudLifecycleMessageZeroHitDay2 || kind == CloudLifecycleMessageZeroHitDay6
		if rescue && (c.activated || now.Sub(c.created) < 48*time.Hour || (kind == CloudLifecycleMessageZeroHitDay6 && now.Sub(c.created) < 6*24*time.Hour)) {
			continue
		}
		if !rescue && kind != CloudLifecycleMessageWelcome && !CloudSubscriptionStatusIsFree(c.SubscriptionStatus) {
			continue
		}
		var monthly int64
		if c.sites > 0 {
			store := m.shared
			if m.dataPlaneEnabled {
				store, err = m.ForTenant(ctx, c.TenantID)
				if err != nil {
					return nil, err
				}
			}
			query := `SELECT s.id,s.domain,a.first_hit_at FROM site_activity_summary a JOIN sites s ON s.id=a.site_id WHERE a.first_hit_at IS NOT NULL`
			var args []any
			if store == m.shared {
				query += ` AND a.site_id IN (SELECT site_id FROM site_tenants WHERE tenant_id=?)`
				args = append(args, c.TenantID)
			}
			query += ` ORDER BY a.first_hit_at,s.domain,s.id LIMIT 1`
			err = store.db.QueryRowContext(ctx, query, args...).Scan(&c.SiteID, &c.SiteDomain, &c.FirstHitAt)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if kind == CloudLifecycleMessageFreeLimitReminder {
				monthly, err = store.CurrentMonthlyHits(ctx, c.TenantID, now)
				if err != nil {
					return nil, err
				}
			}
		}
		switch kind {
		case CloudLifecycleMessageZeroHitDay2, CloudLifecycleMessageZeroHitDay6:
			if !c.FirstHitAt.IsZero() {
				continue
			}
		case CloudLifecycleMessageFreeLimitReminder:
			c.MonthlyLimitNear = monthly >= CloudFreePlanMonthlyHitLimit*8/10
			if c.sites < CloudFreePlanSiteLimit && c.members < CloudFreePlanMemberLimit && !c.MonthlyLimitNear {
				continue
			}
			exempt, err := m.shared.IsTeamOwnedByOperator(ctx, c.TenantID)
			if err != nil {
				return nil, err
			}
			if exempt {
				continue
			}
		case CloudLifecycleMessageFreeRetentionReminder:
			if c.FirstHitAt.IsZero() || c.FirstHitAt.After(now.AddDate(0, 0, -14)) {
				continue
			}
		case CloudLifecycleMessageFreeRetentionPreTrim:
			if c.FirstHitAt.After(now.AddDate(0, 0, -(CloudFreePlanRetentionDays-CloudRetentionPreTrimLeadDays))) || !c.FirstHitAt.After(now.AddDate(0, 0, -CloudFreePlanRetentionDays)) {
				continue
			}
		}
		result = append(result, c.CloudLifecycleRecipient)
	}
	slices.SortFunc(result, func(a, b CloudLifecycleRecipient) int {
		if a.FirstHitAt.IsZero() != b.FirstHitAt.IsZero() {
			if a.FirstHitAt.IsZero() {
				return 1
			}
			return -1
		}
		if diff := a.FirstHitAt.Compare(b.FirstHitAt); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Email, b.Email)
	})
	return result[:min(len(result), limit)], nil
}
