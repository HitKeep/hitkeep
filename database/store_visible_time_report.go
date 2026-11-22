package database

import (
	"context"
	"fmt"
	"uuid"

	"hitkeep/api"
)

// GetVisibleTimePages ranks the busiest tracked paths, retaining views without a sample.
func (s *Store) GetVisibleTimePages(ctx context.Context, params api.AnalyticsParams) ([]api.VisibleTimePage, error) {
	filterSQL, filterArgs := buildHitFilters(params.Filters, "h")
	sessionSQL, sessionArgs, err := s.buildSessionFilter(ctx, params, "h")
	if err != nil {
		return nil, err
	}
	args := append([]any{uuid.Nil().String(), params.SiteID, params.Start, params.End}, filterArgs...)
	args = append(args, sessionArgs...)

	//nolint:gosec // Filter and cohort clauses come from fixed builders with bound values.
	query := fmt.Sprintf(`
		WITH pageviews AS (
			SELECT DISTINCT h.site_id, h.path, h.page_id,
				CASE WHEN h.page_id IS NULL OR h.page_id = ?::UUID THEN h.id END AS legacy_hit_id
			FROM hits h
			WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s%s
		)
		SELECT p.path, AVG(v.visible_ms), COUNT(v.visible_ms), COUNT(*) AS tracked_views
		FROM pageviews p
		LEFT JOIN page_visible_time v
			ON v.site_id = p.site_id AND v.page_id = p.page_id AND p.legacy_hit_id IS NULL
		GROUP BY p.path
		ORDER BY tracked_views DESC, p.path
		LIMIT 10
	`, filterSQL, sessionSQL)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query visible time by page: %w", err)
	}
	defer rows.Close()

	pages := []api.VisibleTimePage{}
	for rows.Next() {
		var page api.VisibleTimePage
		if err := rows.Scan(&page.Path, &page.AvgVisibleMS, &page.MeasuredViews, &page.TrackedViews); err != nil {
			return nil, fmt.Errorf("scan visible time by page: %w", err)
		}
		pages = append(pages, page)
	}
	return pages, rows.Err()
}
