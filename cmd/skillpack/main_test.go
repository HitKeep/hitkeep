package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"hitkeep/skills"
)

func TestArtifactsMatchRuntime(t *testing.T) {
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	for _, output := range []string{first, second} {
		if err := run(output, "https://hitkeep.com/skills/v2.16.0"); err != nil {
			t.Fatal(err)
		}
	}
	var index skills.DiscoveryIndex
	raw, err := os.ReadFile(filepath.Join(first, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Skills) != 5 || len(entries) != 6 {
		t.Fatalf("inventory: %d skills, %d files", len(index.Skills), len(entries))
	}
	for _, skill := range index.Skills {
		expected, err := skills.PublicArchive(skill.Name)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := os.ReadFile(filepath.Join(first, skill.Name+".zip"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, expected) || skill.Digest != fmt.Sprintf("sha256:%x", sha256.Sum256(actual)) {
			t.Fatal("archive differs from runtime")
		}
		if skill.URL != "https://hitkeep.com/skills/v2.16.0/"+skill.Name+".zip" {
			t.Fatalf("URL: %s", skill.URL)
		}
	}
	for _, entry := range entries {
		a, _ := os.ReadFile(filepath.Join(first, entry.Name()))
		b, _ := os.ReadFile(filepath.Join(second, entry.Name()))
		if !bytes.Equal(a, b) {
			t.Fatalf("non-deterministic %s", entry.Name())
		}
	}
	if err := run(first, "https://hitkeep.com/skills/v2.16.0"); err == nil {
		t.Fatal("overwrote existing output")
	}
}

func TestRejectInvalidBaseBeforeWriting(t *testing.T) {
	for _, base := range []string{"", "http://example.test", "https://user:password@example.test", "https://example.test?query", "https://example.test?", "https://example.test/#fragment"} {
		output := filepath.Join(t.TempDir(), "output")
		if err := run(output, base); err == nil {
			t.Fatalf("accepted %s", base)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("invalid input wrote output")
		}
	}
}
