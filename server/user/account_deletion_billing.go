//go:build billing

package user

import (
	"context"
	"hitkeep/api"
	"hitkeep/server/cloud"
)

func (h *handler) stopAccountDeletionRenewals(ctx context.Context, team api.AccountDeletionTeam, attempt string) error {
	return cloud.StopAccountDeletionRenewals(ctx, h.ctx, team, attempt)
}
