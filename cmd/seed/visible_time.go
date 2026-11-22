package main

import (
	"context"

	"github.com/google/uuid"

	"hitkeep/database"
)

func seedVisibleTime(ctx context.Context, store *database.Store, siteID uuid.UUID) error {
	return store.Exec(ctx, `
		INSERT INTO page_visible_time (site_id, page_id, visible_ms)
		SELECT site_id, page_id, CASE WHEN sample_bucket = 0 THEN 0 ELSE 2000 + sample_bucket * 700 END
		FROM (
			SELECT site_id, page_id, hash(path, timestamp) % 43 AS sample_bucket
			FROM hits
			WHERE site_id = ?
		)
		WHERE sample_bucket % 4 <> 1
		ON CONFLICT (site_id, page_id) DO NOTHING
	`, siteID)
}
