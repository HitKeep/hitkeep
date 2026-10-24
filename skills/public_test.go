package skills

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPublicSkillsUseCanonicalBytesAndRejectOtherFiles(t *testing.T) {
	procedures := AnalyticsProcedures()
	if len(procedures) != 5 {
		t.Fatalf("product skills = %d, want 5", len(procedures))
	}
	for _, procedure := range procedures {
		for _, suffix := range []string{"SKILL.md", "references/procedure.md"} {
			name := procedure.Name + "/" + suffix
			got, err := PublicFile(name)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("public %s differs from canonical file", name)
			}
		}
	}
	for _, name := range []string{"../AGENTS.md", "hitkeep-analytics/../embed.go", "hitkeep-development/SKILL.md", ".agents/skills/hitkeep-development/SKILL.md", "hitkeep-analytics/agents/openai.yaml", "embed.go"} {
		if _, err := PublicFile(name); err == nil {
			t.Errorf("exposed %s", name)
		}
	}
	if strings.Contains(EmbeddedAnalyticsProcedurePack(), "name: hitkeep-") {
		t.Fatal("external adapter entered Ask AI")
	}
}
