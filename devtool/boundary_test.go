package devtool

import (
	"strings"
	"testing"
)

func TestRejectDeveloperDependencies(t *testing.T) {
	if err := rejectDeveloperDependencies("self-hosted", "standard/library\nhitkeep/server\n"); err != nil {
		t.Fatalf("runtime-only dependency graph was rejected: %v", err)
	}
	for _, dependency := range []string{
		"hitkeep/devtool",
		"hitkeep/devtool/cli",
		"hitkeep/devtool/devmcp",
		"hitkeep/devtool/cli",
	} {
		if err := rejectDeveloperDependencies("cloud", dependency+"\n"); err == nil {
			t.Fatalf("developer dependency %q was accepted", dependency)
		}
	}
}

func TestBoundedCommandOutputPreservesFinalError(t *testing.T) {
	const finalError = "missing dependency: final diagnostic"
	output := []byte(strings.Repeat("go: downloading module\n", 500) + finalError)
	got := boundedCommandOutput(output)
	if len(got) > 4096 || !strings.HasSuffix(got, finalError) {
		t.Fatalf("bounded output lost final error: %q", got)
	}
	if got := boundedCommandOutput([]byte("  short diagnostic\n")); got != "short diagnostic" {
		t.Fatalf("short output = %q", got)
	}
	if got := boundedCommandOutput([]byte("token=" + strings.Repeat("private", 1000) + "\n" + finalError)); strings.Contains(got, "private") {
		t.Fatalf("bounded output exposed part of a credential: %q", got)
	}
}
