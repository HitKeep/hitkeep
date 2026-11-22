package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"hitkeep/api"
)

type pageVisibleTimeKey struct {
	siteID uuid.UUID
	pageID uuid.UUID
}

// ApplyPageVisibleTime serializes the write with site transfer and deletion.
// An occupied lifecycle lock defers the batch to the consumer's retry budget.
func (m *TenantStoreManager) ApplyPageVisibleTime(ctx context.Context, store *Store, samples []api.PageVisibleTimeSample) ([]bool, error) {
	if !m.shared.lifecycleMu.TryLock() {
		return make([]bool, len(samples)), nil
	}
	defer m.shared.lifecycleMu.Unlock()
	return store.applyPageVisibleTime(ctx, samples)
}

// applyPageVisibleTime returns a flag for each sample whose pageview exists.
// A duplicate or lower value for a matched pageview requires no storage write.
func (s *Store) applyPageVisibleTime(ctx context.Context, samples []api.PageVisibleTimeSample) ([]bool, error) {
	matched := make([]bool, len(samples))
	if len(samples) == 0 || !s.visibleTimeMu.TryLock() {
		return matched, nil
	}
	defer s.visibleTimeMu.Unlock()

	maximums := make(map[pageVisibleTimeKey]int, len(samples))
	lookupArgs := make([]any, 0, len(samples)*2)
	for _, sample := range samples {
		if sample.SiteID == uuid.Nil || sample.PageID == uuid.Nil || sample.VisibleMS < 0 || sample.VisibleMS > api.MaxPageVisibleTimeMS {
			return nil, fmt.Errorf("invalid page visible time sample")
		}
		key := pageVisibleTimeKey{sample.SiteID, sample.PageID}
		if value, ok := maximums[key]; ok {
			maximums[key] = max(value, sample.VisibleMS)
			continue
		}
		maximums[key] = sample.VisibleMS
		lookupArgs = append(lookupArgs, key.siteID, key.pageID)
	}
	lookupValues := strings.Repeat("(CAST(? AS UUID), CAST(? AS UUID)),", len(lookupArgs)/2)
	// #nosec G202 -- lookupValues consists solely of fixed parameter placeholders.
	rows, err := s.db.QueryContext(ctx, `
		SELECT incoming.site_id, incoming.page_id, old.visible_ms
		FROM (VALUES `+strings.TrimSuffix(lookupValues, ",")+`) incoming(site_id, page_id)
		JOIN sites s ON s.id = incoming.site_id
		LEFT JOIN page_visible_time old ON old.site_id = incoming.site_id AND old.page_id = incoming.page_id
		WHERE EXISTS (SELECT 1 FROM hits h WHERE h.site_id = incoming.site_id AND h.page_id = incoming.page_id)
	`, lookupArgs...)
	if err != nil {
		return nil, fmt.Errorf("match visible time to pageviews: %w", err)
	}
	defer rows.Close()

	existing := make(map[pageVisibleTimeKey]struct{}, len(maximums))
	writeArgs := make([]any, 0, len(maximums)*3)
	for rows.Next() {
		var key pageVisibleTimeKey
		var previous sql.NullInt64
		if err := rows.Scan(&key.siteID, &key.pageID, &previous); err != nil {
			return nil, fmt.Errorf("read matched pageview: %w", err)
		}
		existing[key] = struct{}{}
		visibleMS := maximums[key]
		if !previous.Valid || int64(visibleMS) > previous.Int64 {
			writeArgs = append(writeArgs, key.siteID, key.pageID, visibleMS)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read matched pageviews: %w", err)
	}
	for i, sample := range samples {
		_, matched[i] = existing[pageVisibleTimeKey{sample.SiteID, sample.PageID}]
	}
	if len(writeArgs) == 0 {
		return matched, nil
	}

	writeValues := strings.Repeat("(CAST(? AS UUID), CAST(? AS UUID), CAST(? AS INTEGER)),", len(writeArgs)/3)
	// #nosec G202 -- writeValues consists solely of fixed parameter placeholders.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO page_visible_time (site_id, page_id, visible_ms)
		VALUES `+strings.TrimSuffix(writeValues, ",")+`
		ON CONFLICT (site_id, page_id) DO UPDATE SET visible_ms = excluded.visible_ms
		WHERE excluded.visible_ms > page_visible_time.visible_ms
	`, writeArgs...); err != nil {
		return nil, fmt.Errorf("upsert page visible time: %w", err)
	}
	return matched, nil
}

// WithPageVisibleTimeRetention keeps a sample from changing between the hit's
// archive export and its deletion. Stats reset takes the same lock.
func (s *Store) WithPageVisibleTimeRetention(fn func() error) error {
	s.visibleTimeMu.Lock()
	defer s.visibleTimeMu.Unlock()
	return fn()
}
