package ingest

import (
	"errors"

	"github.com/google/uuid"
)

// A site namespace prevents one integration's retry key from suppressing another site's event.
func serverEventID(siteID uuid.UUID, key string) (uuid.UUID, error) {
	if key == "" {
		return uuid.Nil, nil
	}
	if len(key) > 128 {
		return uuid.Nil, errors.New("event ID too long")
	}
	for _, char := range key {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == ':' || char == '-' {
			continue
		}
		return uuid.Nil, errors.New("invalid event ID")
	}
	return uuid.NewSHA1(siteID, []byte(key)), nil
}
