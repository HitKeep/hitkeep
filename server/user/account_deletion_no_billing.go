//go:build !billing

package user

import (
	"context"
	"hitkeep/api"
	"hitkeep/database"
)

func (h *handler) stopAccountDeletionRenewals(_ context.Context, _ api.AccountDeletionTeam, _ string) error {
	return database.ErrAccountDeletionUnavailable
}
