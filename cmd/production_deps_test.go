package hitkeepcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionCommandDoesNotDependOnGenerators(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "hitkeep/cmd/hitkeep").CombinedOutput()
	if err != nil {
		t.Fatalf("go list production deps: %v\n%s", err, out)
	}

	deps := map[string]bool{}
	for dep := range strings.FieldsSeq(string(out)) {
		deps[dep] = true
	}
	for _, unwanted := range []string{
		"hitkeep/cmd/ipmeta-generate",
		"hitkeep/ipmeta/ipmetagen",
		"github.com/DataDog/zstd",
		"github.com/ip2location/ip2location-go/v9",
		"lukechampine.com/uint128",
	} {
		if deps[unwanted] {
			t.Fatalf("production command must not depend on generator package %s", unwanted)
		}
	}
}

func TestSpamFeedPackageDoesNotDependOnDatabase(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "hitkeep/blocking/spamfeed").CombinedOutput()
	if err != nil {
		t.Fatalf("go list spam feed deps: %v\n%s", err, out)
	}
	for dep := range strings.FieldsSeq(string(out)) {
		if dep == "hitkeep/database" || strings.Contains(dep, "duckdb") {
			t.Fatalf("spam feed package must not depend on %s", dep)
		}
	}
}

func TestMaintenanceDependencyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		path      string
		forbidden []string
	}{
		{path: "hitkeep/cmd/data-refresh", forbidden: []string{"hitkeep/cmd", "hitkeep/server", "hitkeep/database", "github.com/duckdb/duckdb-go/v2"}},
		{path: "hitkeep/duckdbextensions/update", forbidden: []string{"hitkeep/ipmeta/ipmetagen", "github.com/DataDog/zstd", "github.com/ip2location/ip2location-go/v9"}},
	} {
		out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", tc.path).CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", tc.path, err, out)
		}
		for dep := range strings.FieldsSeq(string(out)) {
			for _, unwanted := range tc.forbidden {
				if dep == unwanted || (unwanted != "hitkeep/cmd" && strings.HasPrefix(dep, unwanted+"/")) {
					t.Fatalf("%s must not depend on %s", tc.path, dep)
				}
			}
		}
	}
}

func TestProductionBinaryDoesNotContainGeneratorTokenPlumbing(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "hitkeep")
	out, err := exec.CommandContext(t.Context(), "go", "build", "-o", binaryPath, "hitkeep/cmd/hitkeep").CombinedOutput()
	if err != nil {
		t.Fatalf("build production command: %v\n%s", err, out)
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read production binary: %v", err)
	}

	for _, unwanted := range []string{
		"cmd/ipmeta-generate",
		"ipmetagen",
		"ip2location-go",
		"IP2LOCATION_DOWNLOAD_TOKEN",
		"DB3LITEBINIPV6",
		"DBASNLITEBINIPV6",
	} {
		if bytes.Contains(binary, []byte(unwanted)) {
			t.Fatalf("production binary must not contain generator/token string %q", unwanted)
		}
	}
}

func TestDataRefreshWorkflowContracts(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "data-refresh.yml"))
	if err != nil {
		t.Fatalf("read data refresh workflow: %v", err)
	}
	contents := string(workflow)
	for _, required := range []string{
		"workflow_dispatch:",
		"persist-credentials: false",
		"fail-fast: false",
		"cancel-in-progress: false",
		"target: ai-agents",
		"target: spam",
		"target: ipmeta",
		"target: duckdb",
		"run: go run ./cmd/data-refresh ai-agents",
		"run: go run ./cmd/data-refresh spam",
		"run: go run ./cmd/data-refresh ipmeta",
		"IP2LOCATION_DOWNLOAD_TOKEN: ${{ secrets.IP2LOCATION_DOWNLOAD_TOKEN }}",
		"run: go run ./cmd/data-refresh duckdb",
		"run: go run ./duckdbextensions/update -check",
		"GH_TOKEN: ${{ steps.automation-token.outputs.token }}",
		"gh auth setup-git",
		"timeout-minutes: 120",
		"git status --porcelain=v1 -z --untracked-files=all",
		"--force-with-lease=refs/heads/$BRANCH:",
		"--body-file",
	} {
		if !strings.Contains(contents, required) {
			t.Fatalf("data refresh workflow must contain %q", required)
		}
	}
	for _, forbidden := range []string{
		"-ip2location-token",
		"git add ipmeta",
		"git push --force origin",
	} {
		if strings.Contains(contents, forbidden) {
			t.Fatalf("data refresh workflow must not contain %q", forbidden)
		}
	}
	beforeMint, mintAndPublication, ok := strings.Cut(contents, "      - name: Mint refresh publication token\n")
	if !ok {
		t.Fatal("refresh must mint its token immediately before publication")
	}
	mint, publication, ok := strings.Cut(mintAndPublication, "      - name: Open refresh pull request\n")
	if !ok || strings.Contains(mint, "\n      - ") {
		t.Fatal("no generation or test step may run between token minting and publication")
	}
	for _, command := range []string{
		"go test ./aianalytics ./database",
		"go test ./blocking",
		"go test ./cmd/ipmeta-generate ./ipmeta/refresh ./ipmeta ./ipmeta/ipmetagen",
		"go test ./duckdbextensions ./database ./takeout",
	} {
		if !strings.Contains(beforeMint, command) {
			t.Fatalf("token must be minted after %q", command)
		}
	}
	for _, required := range []string{
		"if: env.SELECTED == 'true'",
		"id: automation-token",
		"uses: actions/create-github-app-token@",
		"client-id: ${{ vars.HITKEEP_AUTOMATION_APP_CLIENT_ID }}",
		"private-key: ${{ secrets.AUTOMATION_APP_PRIVATE_KEY }}",
		"owner: ${{ github.repository_owner }}",
		"repositories: ${{ github.event.repository.name }}",
		"permission-contents: write",
		"permission-pull-requests: write",
	} {
		if !strings.Contains(mint, required) {
			t.Fatalf("publication token must contain %q", required)
		}
	}
	if strings.Count(mint, "permission-") != 2 || strings.Contains(contents, "skip-token-revoke:") || strings.Contains(mint, "app-id:") {
		t.Fatal("refresh token must use client-id, only Contents/PR write, and automatic revocation")
	}
	for _, forbidden := range []string{"AUTOMATION_APP_PRIVATE_KEY", "steps.automation-token.outputs.token"} {
		if strings.Contains(beforeMint, forbidden) {
			t.Fatalf("generation and tests must not receive publication credential %q", forbidden)
		}
	}
	if strings.Contains(contents, "secrets.GHT") || strings.Count(contents, "AUTOMATION_APP_PRIVATE_KEY") != 1 ||
		strings.Count(contents, "steps.automation-token.outputs.token") != 1 ||
		!strings.Contains(publication, "GH_TOKEN: ${{ steps.automation-token.outputs.token }}") {
		t.Fatal("App credential must be used only for minting and its token only for publication")
	}
}

func TestReleaseAutomationWorkflowContracts(t *testing.T) {
	for _, tc := range []struct {
		file      string
		start     string
		end       string
		required  []string
		forbidden []string
	}{
		{
			file: "release.yml", start: "  release-please:\n", end: "  resolve-candidate:\n",
			required: []string{
				"permissions: {}",
				"uses: actions/create-github-app-token@",
				"client-id: ${{ vars.HITKEEP_AUTOMATION_APP_CLIENT_ID }}",
				"private-key: ${{ secrets.AUTOMATION_APP_PRIVATE_KEY }}",
				"owner: ${{ github.repository_owner }}",
				"repositories: ${{ github.event.repository.name }}",
				"permission-contents: write",
				"permission-pull-requests: write",
				"permission-issues: write",
				"token: ${{ steps.automation-token.outputs.token }}",
				"release_created: ${{ steps.release.outputs.release_created }}",
				"tag_name: ${{ steps.release.outputs.tag_name }}",
				"config-file: release-please-config.json",
				"manifest-file: .release-please-manifest.json",
			},
			forbidden: []string{"secrets.GHT", "app-id:", "skip-token-revoke:"},
		},
		{
			file: "snapshot.yml", start: "    secrets:\n",
			required:  []string{"DOCKERHUB_USERNAME:", "DOCKERHUB_TOKEN:"},
			forbidden: []string{"secrets.GHT", "      token:"},
		},
		{
			file: "pipeline.yml", start: "    secrets:\n", end: "jobs:\n",
			forbidden: []string{"      token:"},
		},
		{
			file: "pipeline.yml", start: "  upload-release-binaries:\n", end: "  build-and-push-image:\n",
			required: []string{
				"contents: write",
				"GITHUB_TOKEN: ${{ github.token }}",
				"gh release create",
				"gh release upload",
			},
			forbidden: []string{"secrets.token", "secrets.GHT"},
		},
	} {
		t.Run(tc.file+tc.start, func(t *testing.T) {
			workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			_, section, ok := strings.Cut(string(workflow), tc.start)
			if !ok {
				t.Fatalf("missing section %q", tc.start)
			}
			if tc.end != "" {
				section, _, ok = strings.Cut(section, tc.end)
				if !ok {
					t.Fatalf("missing section end %q", tc.end)
				}
			}
			for _, required := range tc.required {
				if !strings.Contains(section, required) {
					t.Errorf("workflow section must contain %q", required)
				}
			}
			for _, forbidden := range tc.forbidden {
				if strings.Contains(section, forbidden) {
					t.Errorf("workflow section must not contain %q", forbidden)
				}
			}
		})
	}
}
