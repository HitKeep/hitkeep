//go:build !billing

package ingest

import (
	"context"

	"github.com/google/uuid"
)

type monthlyQuota struct{}

func (*monthlyQuota) drop(context.Context, uuid.UUID) bool { return false }

func (h *handler) dropMonthlyHit(ctx context.Context, siteID uuid.UUID) bool {
	return h.monthlyQuota.drop(ctx, siteID)
}
