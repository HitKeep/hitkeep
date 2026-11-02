package skills

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
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

func TestPublicSkillArchivesAndIndex(t *testing.T) {
	index, err := PublicIndex("https://example.test/tenant")
	if err != nil {
		t.Fatal(err)
	}
	if index.Schema != "https://schemas.agentskills.io/discovery/0.2.0/schema.json" || len(index.Skills) != 5 {
		t.Fatalf("index: %+v", index)
	}
	for _, entry := range index.Skills {
		data, err := PublicArchive(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		repeated, err := PublicArchive(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, repeated) || entry.Digest != fmt.Sprintf("sha256:%x", sha256.Sum256(data)) || entry.Type != "archive" || entry.Description == "" || entry.URL != "https://example.test/tenant/skills/"+entry.Name+"/skill.zip" {
			t.Fatalf("artifact: %+v", entry)
		}
		archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		if len(archive.File) != 2 {
			t.Fatalf("archive has %d files", len(archive.File))
		}
		for i, name := range []string{"SKILL.md", "references/procedure.md"} {
			file := archive.File[i]
			if file.Name != name || !file.Mode().IsRegular() {
				t.Fatalf("unsafe archive file: %+v", file)
			}
			reader, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			want, err := PublicFile(entry.Name + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("archive changed canonical bytes")
			}
		}
	}
	for _, name := range []string{"../hitkeep-analytics", "hitkeep-development", "hitkeep-analytics/../hitkeep-analytics"} {
		if _, err := PublicArchive(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}
