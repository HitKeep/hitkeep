package devtool

import (
	"bytes"
	json "encoding/json/v2"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSkillMetadataRejectsAdditionalFrontmatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	body := "---\nname: example\ndescription: 'Example skill.'\nlicense: MIT\n---\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSkillMetadata(path); err == nil {
		t.Fatal("unsupported skill metadata was accepted")
	}
}

func TestSkillMetadataRejectsUnclosedFrontmatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	body := "---\nname: example\ndescription: 'Example skill.'\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSkillMetadata(path); err == nil {
		t.Fatal("unclosed skill frontmatter was accepted")
	}
}

func TestRepositoryContributorSkillContracts(t *testing.T) {
	root := repositoryRoot(t)
	required := map[string][]string{
		"hitkeep-development": {"AGENTS.md", "callable `hk_*` tools", "explicit user approval", "registration, startup, workspace routing, or task reload", "./hk mcp manifest --output json", "configured fallback or explicit workspace selector", "hk_context", "hk_doctor", "hk_dev_start", "hk_run_start", "hk_screenshot", "hk_run_status", "--detach --output json", "$hitkeep-workspace", "$hitkeep-qa"},
		"hitkeep-workspace":   {"AGENTS.md", "whenever they are callable", "explicit user approval", "hk_context", "hk_run_start", "hk_run_status", "cursor-addressed", "hk_dev_start", "hk_dev_stop", "hk_screenshot", "hk_run_cancel", "--output json"},
		"hitkeep-qa":          {"AGENTS.md", "whenever it is callable", "explicit user approval", "hk_qa_plan", "plan_id", "hk_run_start", "hk_run_status", "hk_run_cancel", "complete", "--detach --output json"},
		"hitkeep-i18n":        {"AGENTS.md", "$hitkeep-qa", "frontend/dashboard/public/i18n", "every currently supported locale"},
	}
	triggerTerms := map[string][]string{
		"hitkeep-development": {"code", "UI", "database", "documentation", "setup", "local development", "builds"},
		"hitkeep-workspace":   {"worktrees", "ports", "concurrent agents", "handoff"},
		"hitkeep-qa":          {"quality gates", "complete", "CI parity", "failed"},
		"hitkeep-i18n":        {"user-visible", "Transloco", "language switching", "localized layout"},
	}
	for _, name := range contributorSkills {
		skillPath := filepath.Join(root, ".agents", "skills", name, "SKILL.md")
		assertSkillContract(t, skillPath, name, required[name], triggerTerms[name])
	}
}

func TestRepositoryProductAnalyticsSkillContracts(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range productAnalyticsSkills {
		skillPath := filepath.Join(root, "skills", name, "SKILL.md")
		assertSkillContract(t, skillPath, name, []string{"references/procedure.md", "production HitKeep MCP"}, nil)
		procedure, err := os.ReadFile(filepath.Join(root, "skills", name, "references", "procedure.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"MCP", "API client token", "./hk", "hk_doctor", "TranslocoPipe"} {
			if strings.Contains(string(procedure), forbidden) {
				t.Errorf("%s transport-neutral procedure contains %q", name, forbidden)
			}
		}
	}
}

func TestRepositorySkillLayout(t *testing.T) {
	root := repositoryRoot(t)
	if err := ValidateSkillLayout(root); err != nil {
		t.Fatal(err)
	}
	for path, expected := range map[string]int{
		filepath.Join(root, "skills"): len(productAnalyticsSkills),
	} {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		directories := 0
		for _, entry := range entries {
			if entry.IsDir() {
				directories++
			}
		}
		if directories != expected {
			t.Errorf("skill directory count under %s = %d, want %d", path, directories, expected)
		}
	}
}

func assertSkillContract(t *testing.T, skillPath, name string, required, triggerTerms []string) {
	t.Helper()
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	metadata, err := readSkillMetadata(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != name {
		t.Errorf("%s metadata name = %q", name, metadata.Name)
	}
	for _, value := range required {
		if !strings.Contains(body, value) {
			t.Errorf("%s is missing contract %q", name, value)
		}
	}
	for _, value := range triggerTerms {
		if !strings.Contains(metadata.Description, value) {
			t.Errorf("%s description is missing trigger %q", name, value)
		}
	}
	for _, forbidden := range []string{"hashicorpmetrics", "GOFLAGS=", "make dev-seed", "demo@example.com", "demo1234"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("%s contains mutable or sensitive fact %q", name, forbidden)
		}
	}

	metadataPath := filepath.Join(filepath.Dir(skillPath), "agents", "openai.yaml")
	openAI, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	short := yamlQuotedValue(string(openAI), "short_description")
	if len(short) < 25 || len(short) > 64 {
		t.Errorf("%s short_description length = %d", name, len(short))
	}
	defaultPrompt := yamlQuotedValue(string(openAI), "default_prompt")
	if !strings.Contains(defaultPrompt, "$"+name) {
		t.Errorf("%s default_prompt does not invoke the skill", name)
	}
	if name == "hitkeep-development" && !strings.Contains(defaultPrompt, "MCP") {
		t.Errorf("%s default_prompt does not reinforce MCP-first execution", name)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	t.Fatal("resolve repository root")
	return ""
}

func yamlQuotedValue(raw, key string) string {
	prefix := "  " + key + ": \""
	for line := range strings.Lines(raw) {
		if after, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSuffix(after, "\"\n")
		}
	}
	return ""
}

func TestAgentPluginFixturesStageCanonicalSkills(t *testing.T) {
	root := repositoryRoot(t)
	for _, platform := range []string{"openai", "claude"} {
		t.Run(platform, func(t *testing.T) {
			stage := t.TempDir()
			fixture := filepath.Join(root, "tests", "agent-plugins", platform)
			if err := os.CopyFS(stage, os.DirFS(fixture)); err != nil {
				t.Fatal(err)
			}
			expected := map[string][]byte{}
			pluginFiles(t, fixture, "", expected)
			for _, file := range []struct{ source, destination string }{
				{filepath.Join(root, "tests", "agent-plugins", "README.md"), "README.md"},
				{filepath.Join(root, "LICENSE"), "LICENSE"},
			} {
				body, err := os.ReadFile(file.source)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(stage, file.destination), body, 0o644); err != nil {
					t.Fatal(err)
				}
				expected[file.destination] = body
			}
			for _, name := range productAnalyticsSkills {
				source := filepath.Join(root, "skills", name)
				relative := filepath.Join("skills", name)
				pluginFiles(t, source, relative, expected)
				if err := os.CopyFS(filepath.Join(stage, relative), os.DirFS(source)); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(stage, relative, "references", "procedure.md")); err != nil {
					t.Fatalf("missing canonical procedure for %s: %v", name, err)
				}
			}
			if err := validatePluginFiles(stage, expected); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Join(stage, "skills"))
			if err != nil || len(entries) != len(productAnalyticsSkills) {
				t.Fatalf("staged skill identities differ from the canonical pack: %v", err)
			}

			manifestPath := "plugin.json"
			if platform == "claude" {
				manifestPath = filepath.Join(".claude-plugin", "plugin.json")
			}
			manifest := pluginObject(t, filepath.Join(stage, manifestPath))
			description, described := manifest["description"].(string)
			if manifest["name"] != "hitkeep-analytics" || manifest["version"] != "0.0.0" || manifest["license"] != "MIT" || !described || strings.TrimSpace(description) == "" {
				t.Fatal("fixture identity, version or license is invalid")
			}
			if !reflect.DeepEqual(manifest["author"], map[string]any{"name": "HitKeep"}) {
				t.Fatal("fixture author must not contain invented account or contact data")
			}
			if platform == "claude" {
				pluginKeys(t, manifest, "name", "version", "description", "author", "license", "skills")
				path, err := containedPluginPath(stage, manifest["skills"].(string))
				if err != nil || path != filepath.Join(stage, "skills") {
					t.Fatalf("Claude skills path escapes the staged pack: %v", err)
				}
				if _, err := os.Stat(filepath.Join(stage, ".mcp.json")); !os.IsNotExist(err) {
					t.Fatal("Claude skills-only fixture must leave connector setup separate")
				}
			} else {
				pluginKeys(t, manifest, "$schema", "name", "version", "description", "author", "license", "extensions")
				if manifest["$schema"] != "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" {
					t.Fatal("portable plugin schema identity changed")
				}
				extensions := manifest["extensions"].(map[string]any)
				pluginKeys(t, extensions, "com.openai")
				openai := extensions["com.openai"].(map[string]any)
				pluginKeys(t, openai, "interface")
				presentation := openai["interface"].(map[string]any)
				pluginKeys(t, presentation, "displayName", "shortDescription", "capabilities")
				for _, field := range []string{"displayName", "shortDescription"} {
					label, ok := presentation[field].(string)
					if !ok || strings.TrimSpace(label) == "" {
						t.Fatalf("OpenAI presentation %s must be nonempty text", field)
					}
				}
				if !reflect.DeepEqual(presentation["capabilities"], []any{"Read"}) {
					t.Fatal("fixture must advertise only read capability")
				}
				config := pluginObject(t, filepath.Join(stage, "mcp.json"))
				pluginKeys(t, config, "mcpServers")
				servers := config["mcpServers"].(map[string]any)
				pluginKeys(t, servers, "hitkeep")
				server := servers["hitkeep"].(map[string]any)
				pluginKeys(t, server, "type", "url", "extensions")
				if server["type"] != "streamable-http" || server["url"] != "https://example.com/mcp" {
					t.Fatal("offline fixture requires the reserved example HTTPS resource")
				}
				want := map[string]any{"com.openai": map[string]any{"auth": map[string]any{
					"type": "oauth", "client": map[string]any{"mode": "cimd"},
					"baseScopes": []any{"sites:read", "analytics:read"},
				}}}
				if !reflect.DeepEqual(server["extensions"], want) {
					t.Fatal("OpenAI fixture must use discovery-based public CIMD and exact MCP read scopes without credentials")
				}
			}

			// A copied contributor or executable file must never silently join a package.
			if err := os.WriteFile(filepath.Join(stage, "unexpected-hook.sh"), []byte("unexpected"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := validatePluginFiles(stage, expected); err == nil {
				t.Fatal("package content outside the canonical allowlist was accepted")
			}
		})
	}
}

func TestAgentPluginPathsRejectEscapes(t *testing.T) {
	for _, path := range []string{"../skills", "/skills", "./skills/../../outside", "./C:/outside", `./skills\outside`} {
		if _, err := containedPluginPath(t.TempDir(), path); err == nil {
			t.Errorf("unsafe plugin path accepted: %q", path)
		}
	}
}

func containedPluginPath(root, path string) (string, error) {
	relative, ok := strings.CutPrefix(path, "./")
	relative = strings.TrimSuffix(relative, "/")
	if !ok || !fs.ValidPath(relative) || strings.ContainsAny(relative, `:\`) {
		return "", fmt.Errorf("plugin path must stay within its root: %q", path)
	}
	return filepath.Join(root, filepath.FromSlash(relative)), nil
}

func pluginObject(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		t.Fatalf("invalid plugin JSON at %s: %v", path, err)
	}
	return object
}

func pluginKeys(t *testing.T, object map[string]any, allowed ...string) {
	t.Helper()
	if len(object) != len(allowed) {
		t.Fatalf("unexpected plugin fields: got %v, want %v", object, allowed)
	}
	for key := range object {
		if !slices.Contains(allowed, key) {
			t.Fatalf("unsupported plugin field %q", key)
		}
	}
}

func pluginFiles(t *testing.T, source, prefix string, expected map[string][]byte) {
	t.Helper()
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("package source is not a regular file: %s", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err == nil {
			expected[filepath.Join(prefix, relative)] = body
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func validatePluginFiles(root string, expected map[string][]byte) error {
	seen := 0
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staged plugin is not a regular file: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		want, ok := expected[relative]
		if !ok {
			return fmt.Errorf("unexpected plugin content: %s", relative)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(body, want) {
			return fmt.Errorf("staged plugin differs from canonical content: %s", relative)
		}
		seen++
		return nil
	}); err != nil {
		return err
	}
	if seen != len(expected) {
		return fmt.Errorf("staged plugin has %d files, want %d", seen, len(expected))
	}
	return nil
}
