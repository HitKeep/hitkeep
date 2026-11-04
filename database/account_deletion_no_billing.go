//go:build !billing

package database

import (
	"context"

	"github.com/google/uuid"
)

func (s *Store) accountDeletionBilling(_ context.Context, _ uuid.UUID) (string, bool, error) {
	return "", false, nil
}
