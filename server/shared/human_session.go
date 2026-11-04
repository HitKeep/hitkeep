package shared

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// HumanSessionUserAvailable is authoritative for cookie authentication and minting.
func (c *Context) HumanSessionUserAvailable(ctx context.Context, id uuid.UUID) (bool, error) {
	if c == nil || c.Store == nil {
		return false, errors.New("account store unavailable")
	}
	user, err := c.Store.GetUserByID(ctx, id)
	if err != nil || user == nil {
		return false, err
	}
	removing, err := c.Store.AccountRemovalStarted(ctx, id)
	return err == nil && !removing, err
}
