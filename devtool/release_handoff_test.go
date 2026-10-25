package devtool

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestReleaseCandidateAndPublicationFaults(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "python3", "-B", "-m", "unittest", "discover", "-s", "scripts/release", "-p", "test_*.py")
	cmd.Dir = ".."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("release fault proofs: %v\n%s", err, output)
	}
}

func TestReleaseCandidateWaitsForEveryMandatoryCheck(t *testing.T) {
	release := readReleaseWorkflow(t, "release.yml")
	pipeline := readReleaseWorkflow(t, "pipeline.yml")
	for _, required := range []string{"needs: [resolve-candidate, build-release]", "retention-days: 30", "upload_to_release: false", "TESTED_IMAGE_DIGEST", "candidate/release-candidate.json"} {
		if !strings.Contains(release, required) {
			t.Errorf("candidate is missing %q", required)
		}
	}
	for _, job := range []string{"build-configuration", "build-binaries", "build-release-archives", "package-tracker", "package-chart", "upgrade-from-supported-floor", "docs-compatibility"} {
		if !strings.Contains(pipeline, "  "+job+":\n") {
			t.Errorf("missing mandatory pipeline job %s", job)
		}
	}
	if strings.Contains(pipeline, "retention-days: 1\n") || strings.Contains(release, "gh release upload") || strings.Contains(release, "helm push") {
		t.Fatal("stable coordinator must certify recoverable inputs without publishing")
	}
}

func TestPrivilegedPublisherRunsReviewedSourceAndIndependentChannels(t *testing.T) {
	raw := readReleaseWorkflow(t, "release-publish.yml")
	for _, required := range []string{"RELEASE_PUBLICATION_ENABLED == 'true'", "group: stable-publication", "ref: ${{ github.sha }}", "--channel github", "--channel npm", "--channel images", "--channel helm"} {
		if !strings.Contains(raw, required) {
			t.Errorf("publisher missing %q", required)
		}
	}
	for _, forbidden := range []string{"ref: ${{ github.event.workflow_run.head_sha }}", "secrets: inherit", "--clobber", "continue-on-error", "npm ci", "./hk"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("privileged publisher contains %q", forbidden)
		}
	}
}

func readReleaseWorkflow(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../.github/workflows/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
