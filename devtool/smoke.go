package devtool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	json "hitkeep/jsonapi"
)

func (a *App) previousSmokeImage(ctx context.Context, candidate string) (string, error) {
	if explicit := os.Getenv("HITKEEP_PREVIOUS_IMAGE"); explicit != "" {
		return explicit, nil
	}
	// Inspect the built image so remote Docker and explicit build platforms select the matching fixture.
	command := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", candidate) //nolint:gosec // Candidate comes from the closed variant catalog.
	command.Dir = a.workspace.Root
	command.Env = a.commandEnvironment(nil)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("inspect smoke candidate platform: %w", err)
	}
	platform := strings.TrimSpace(string(output))
	if platform != "linux/amd64" && platform != "linux/arm64" {
		return "", fmt.Errorf("unsupported smoke candidate platform %q", platform)
	}
	raw, err := os.ReadFile(filepath.Join(a.workspace.Root, "tests", "fixtures", "release-fixtures.json"))
	if err != nil {
		return "", fmt.Errorf("read supported upgrade fixture: %w", err)
	}
	var manifest struct {
		SchemaVersion         int    `json:"schema_version"`
		Protocol              string `json:"protocol"`
		SupportedUpgradeFloor string `json:"supported_upgrade_floor"`
		Fixtures              []struct {
			PreviousVersion string `json:"previous_version"`
			Platform        string `json:"platform"`
			PreviousImage   string `json:"previous_image"`
		} `json:"fixtures"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("decode supported upgrade fixture: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Protocol != "upgrade-fixture-v1" || !regexp.MustCompile(`^2\.[0-9]+\.[0-9]+$`).MatchString(manifest.SupportedUpgradeFloor) {
		return "", errors.New("unsupported upgrade fixture contract")
	}
	previous := ""
	matches := 0
	for _, fixture := range manifest.Fixtures {
		if fixture.PreviousVersion == manifest.SupportedUpgradeFloor && fixture.Platform == platform {
			previous = fixture.PreviousImage
			matches++
		}
	}
	if matches != 1 {
		return "", fmt.Errorf("expected one supported upgrade fixture for %q on %q, got %d", manifest.SupportedUpgradeFloor, platform, matches)
	}
	reference, digest, pinned := strings.Cut(previous, "@sha256:")
	if !pinned || reference == "" || strings.ContainsAny(reference, " \t\r\n") || len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		return "", errors.New("supported upgrade fixture image must be immutable")
	}
	return previous, nil
}
