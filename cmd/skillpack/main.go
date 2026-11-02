// Command skillpack writes the same public skill artifacts served by HitKeep.
package main

import (
	"encoding/json/v2"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"hitkeep/skills"
)

func main() {
	output := flag.String("output", "", "new output directory")
	base := flag.String("base-url", "", "absolute HTTPS directory containing the released archives")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	if err := run(*output, *base); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(output, base string) error {
	u, err := url.Parse(base)
	if output == "" || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("output and an absolute HTTPS base URL without credentials, query or fragment are required")
	}
	base = strings.TrimRight(base, "/")
	index, err := skills.PublicIndex(base)
	if err != nil {
		return err
	}
	artifacts := make(map[string][]byte, len(index.Skills)+1)
	for i, skill := range index.Skills {
		raw, err := skills.PublicArchive(skill.Name)
		if err != nil {
			return err
		}
		artifacts[skill.Name+".zip"] = raw
		index.Skills[i].URL = base + "/" + skill.Name + ".zip"
	}
	raw, err := json.Marshal(index)
	if err != nil {
		return err
	}
	artifacts["index.json"] = append(raw, '\n')
	// Refuse existing output rather than overwriting a previous release or following a symlink.
	if err := os.Mkdir(output, 0o755); err != nil {
		return fmt.Errorf("create new output directory: %w", err)
	}
	root, err := os.OpenRoot(output)
	if err != nil {
		return err
	}
	defer root.Close()
	for name, raw := range artifacts {
		if err := root.WriteFile(name, raw, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
