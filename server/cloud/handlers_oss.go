//go:build !billing

package cloud

import (
	"net/http"

	"hitkeep/config"
	"hitkeep/server/shared"
)

func Register(_ *http.ServeMux, _ *shared.Context) {}

// PlansAvailable is false in builds without Cloud billing routes.
func PlansAvailable(_ *config.Config) bool { return false }
