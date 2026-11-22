package api

import "github.com/google/uuid"

const MaxPageVisibleTimeMS = 86_400_000

// PageVisibleTimeSample is the internal ingest message after site authorization.
type PageVisibleTimeSample struct {
	SiteID    uuid.UUID `json:"site_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	PageID    uuid.UUID `json:"page_id"`
	VisibleMS int       `json:"visible_ms"`
}
