package skills

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"hitkeep/appurl"
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

// DiscoveryIndex describes the v0.2 artifacts fetched by external skill clients.
type DiscoveryIndex struct {
	Schema string           `json:"$schema"`
	Skills []DiscoverySkill `json:"skills"`
}

type DiscoverySkill struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Digest      string `json:"digest"`
}

// PublicArchive packages only the canonical adapter and its procedure, at the archive root.
func PublicArchive(name string) ([]byte, error) {
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	for _, file := range []string{"SKILL.md", "references/procedure.md"} {
		content, err := PublicFile(name + "/" + file)
		if err != nil {
			return nil, err
		}
		header := &zip.FileHeader{Name: file, Method: zip.Store, Modified: time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)}
		header.SetMode(0644)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err = writer.Write(content); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

// PublicIndex derives descriptions and digests from the exact canonical artifacts.
func PublicIndex(base string) (DiscoveryIndex, error) {
	index := DiscoveryIndex{Schema: "https://schemas.agentskills.io/discovery/0.2.0/schema.json", Skills: []DiscoverySkill{}}
	for _, procedure := range AnalyticsProcedures() {
		adapter, err := PublicFile(procedure.Name + "/SKILL.md")
		if err != nil {
			return index, err
		}
		_, frontmatter, ok := strings.Cut(string(adapter), "---\n")
		if !ok {
			return index, fmt.Errorf("skill %s has no frontmatter", procedure.Name)
		}
		frontmatter, _, ok = strings.Cut(frontmatter, "\n---")
		if !ok {
			return index, fmt.Errorf("skill %s has no frontmatter boundary", procedure.Name)
		}
		var metadata struct {
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
			return index, err
		}
		artifact, err := PublicArchive(procedure.Name)
		if err != nil {
			return index, err
		}
		index.Skills = append(index.Skills, DiscoverySkill{Name: procedure.Name, Type: "archive", Description: metadata.Description, URL: appurl.Path(base, "/skills/"+procedure.Name+"/skill.zip"), Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(artifact))})
	}
	return index, nil
}
