package devtool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDocsScreenshotsCopiesDocsSetAndReadmeSubset(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "capture")
	docs := filepath.Join(root, "hitkeep-docs")
	readme := filepath.Join(root, "repo", ".github", "assets")
	for _, dir := range []string{source, docs} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"dashboard-overview.png", "analytics-ecommerce.png", "analytics-search-console.png", "analytics-ai-visibility.png", "analytics-ai-agents-traffic.png", "feature-ask-ai-answer.png", "mcp.png", "settings-sites.png"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	written, err := syncDocsScreenshots(source, readme, docs, readmeScreenshots)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 15 {
		t.Fatalf("written = %v, want eight docs images and all seven README images", written)
	}
	for _, path := range []string{filepath.Join(docs, "settings-sites.png"), filepath.Join(readme, "dashboard-overview.png"), filepath.Join(readme, "mcp.png"), filepath.Join(readme, "analytics-ai-agents-traffic.png")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(readme, "settings-sites.png")); !os.IsNotExist(err) {
		t.Fatal("only the README subset belongs in .github/assets")
	}

	if _, err := syncDocsScreenshots(source, readme, filepath.Join(root, "missing-docs"), []string{"missing.png"}); err == nil {
		t.Fatal("a missing README screenshot must fail the sync")
	}
}
