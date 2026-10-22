//go:build billing

package cloud

import "hitkeep/config"

// PlansAvailable reports whether the Cloud-only catalog route is implemented and enabled.
func PlansAvailable(conf *config.Config) bool { return conf != nil && conf.CloudHosted }
