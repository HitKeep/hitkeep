package database

import (
	"context"
	"fmt"

	"hitkeep/api"
)

// Match the tracker's whitespace trimming for stored values, including old hits and imports.
func utmValueSQL(column string) string {
	return `NULLIF(regexp_replace(` + column + `, '^[[:space:]\p{Z}\x85\x{FEFF}]+|[[:space:]\p{Z}\x85\x{FEFF}]+$', '', 'g'), '')`
}

func (s *Store) queryUTMRankings(ctx context.Context, params api.AnalyticsParams, truncUnit, filterSQL string, filterArgs []any, stats *api.SiteStats) error {
	// Rank after combining all eligible tracked and imported counts for each named value.
	//nolint:gosec // filterSQL and value expressions come from fixed columns and allowed filters.
	query := fmt.Sprintf(`
		WITH scoped AS (
			SELECT h.utm_source, h.utm_medium, h.utm_campaign, h.utm_content, h.utm_term
			FROM hits h WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
		), tracked AS (
			UNPIVOT scoped ON utm_source, utm_medium, utm_campaign, utm_content, utm_term
			INTO NAME dimension VALUE raw_value
		), counts AS (
			SELECT dimension, raw_value, COUNT(*) AS views FROM tracked GROUP BY dimension, raw_value
			UNION ALL
			SELECT dimension, name, SUM(pageviews) FROM imported_dimension_daily
			WHERE ? AND site_id = ? AND date BETWEEN CAST(? AS DATE) AND CAST(? AS DATE)
				AND dimension IN ('utm_source', 'utm_medium', 'utm_campaign', 'utm_content', 'utm_term')
			GROUP BY dimension, name
		), named AS (
			SELECT dimension, %s AS name, views FROM counts
		)
		SELECT dimension, name, SUM(views) AS total FROM named WHERE name IS NOT NULL
		GROUP BY dimension, name
		QUALIFY ROW_NUMBER() OVER (PARTITION BY dimension ORDER BY total DESC, name) <= 10
		ORDER BY dimension, total DESC, name`, filterSQL, utmValueSQL("raw_value"))
	start, end := fullImportedDates(params.Start, params.End)
	args := append([]any{params.SiteID, params.Start, params.End}, filterArgs...)
	args = append(args, canIncludeImportedSiteAggregates(params, truncUnit), params.SiteID, start, end)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query UTM rankings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dim string
		var metric api.MetricStat
		if err := rows.Scan(&dim, &metric.Name, &metric.Value); err != nil {
			return err
		}
		switch dim {
		case "utm_source":
			stats.TopUTMSources = append(stats.TopUTMSources, metric)
		case "utm_medium":
			stats.TopUTMMediums = append(stats.TopUTMMediums, metric)
		case "utm_campaign":
			stats.TopUTMCampaigns = append(stats.TopUTMCampaigns, metric)
		case "utm_content":
			stats.TopUTMContents = append(stats.TopUTMContents, metric)
		case "utm_term":
			stats.TopUTMTerms = append(stats.TopUTMTerms, metric)
		}
	}
	return rows.Err()
}
