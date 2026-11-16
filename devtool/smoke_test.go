package devtool

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	json "hitkeep/jsonapi"
)

func TestExecuteSmokeResolvesImmutableSupportedFloor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "tests", "fixtures", "release-fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"arm64", "amd64", "override", "missing", "malformed", "schema", "no-match", "conflict", "mutable", "unsupported", "inspect-failure", "cloud"} {
		t.Run(mode, func(t *testing.T) {
			root := initTestRepository(t)
			writeTestToolchainConfig(t, root)
			t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			t.Setenv("HITKEEP_PREVIOUS_IMAGE", "")
			app, err := NewApp(root)
			if err != nil {
				t.Fatal(err)
			}
			paths, err := app.managedToolchainPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(paths.GoExecutable), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(paths.GoExecutable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			platform := "linux/arm64"
			if mode == "amd64" {
				platform = "linux/amd64"
			} else if mode == "unsupported" {
				platform = "linux/386"
			}
			fakeBin := t.TempDir()
			docker := "#!/bin/sh\nif [ \"$1 $2\" = 'image inspect' ]; then\n"
			if mode == "inspect-failure" {
				docker += "exit 1\n"
			} else {
				docker += "printf '%s\\n' '" + platform + "'\n"
			}
			docker += "fi\n"
			if err := os.WriteFile(filepath.Join(fakeBin, "docker"), []byte(docker), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "scripts", "docker-smoke.sh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$HITKEEP_PREVIOUS_IMAGE\" \"$@\" > smoke-command\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(raw, &manifest); err != nil {
				t.Fatal(err)
			}
			fixtures := manifest["fixtures"].([]any)
			want := ""
			for _, value := range fixtures {
				fixture := value.(map[string]any)
				fixture["previous_image"] = "registry.example/" + strings.ReplaceAll(fixture["platform"].(string), "/", "-") + "@sha256:" + strings.Repeat("a", 64)
				if fixture["platform"] == platform && fixture["previous_version"] == manifest["supported_upgrade_floor"] {
					want = fixture["previous_image"].(string)
				}
			}
			switch mode {
			case "override":
				want = "registry.example/explicit@sha256:" + strings.Repeat("a", 64)
				t.Setenv("HITKEEP_PREVIOUS_IMAGE", want)
			case "schema":
				manifest["schema_version"] = 99
			case "no-match":
				manifest["fixtures"] = []any{}
			case "conflict":
				manifest["fixtures"] = append(fixtures, fixtures[1])
			case "mutable":
				fixtures[1].(map[string]any)["previous_image"] = "registry.example/mutable:latest"
			}
			if mode != "missing" && mode != "override" && mode != "cloud" {
				content, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "malformed" {
					content = []byte("{")
				}
				path := filepath.Join(root, "tests", "fixtures", "release-fixtures.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			variant := "self-hosted"
			flag := "--recreate"
			if mode == "cloud" {
				variant, flag, want = "cloud", "--cloud", ""
			}
			err = app.executeSmoke(t.Context(), variant, io.Discard)
			command, readErr := os.ReadFile(filepath.Join(root, "smoke-command"))
			if !slices.Contains([]string{"arm64", "amd64", "override", "cloud"}, mode) {
				if err == nil || !os.IsNotExist(readErr) {
					t.Fatalf("invalid prerequisite reached smoke: err=%v command=%q readErr=%v", err, command, readErr)
				}
				return
			}
			selected, _ := VariantByID(variant)
			expected := strings.Join([]string{want, app.localImageRef(selected), variant, flag, ""}, "\n")
			if err != nil || readErr != nil || string(command) != expected {
				t.Fatalf("smoke command=%q want=%q err=%v readErr=%v", command, expected, err, readErr)
			}
		})
	}
}
