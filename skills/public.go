package skills

import (
	"embed"
	"io/fs"
	"strings"
)

// External adapters are served separately and never enter the Ask AI procedure pack.
//
//go:embed hitkeep-analytics/SKILL.md hitkeep-traffic-diagnosis/SKILL.md hitkeep-ai-visibility-analyst/SKILL.md hitkeep-ecommerce-analyst/SKILL.md hitkeep-tracking-verifier/SKILL.md
var publicAdapters embed.FS

// PublicFile returns only a canonical product adapter or its procedure reference.
func PublicFile(name string) ([]byte, error) {
	for _, procedure := range embeddedAnalyticsProcedureFiles {
		if name == procedure {
			return skillFS.ReadFile(name)
		}
		skill, _, _ := strings.Cut(procedure, "/")
		if name == skill+"/SKILL.md" {
			return publicAdapters.ReadFile(name)
		}
	}
	return nil, fs.ErrNotExist
}
