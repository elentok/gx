// Package skills holds gx's canonical, installable skill bundle (see
// README.md) and validates it: metadata, invocation policy, relative
// references between bundle files, and a representative generated ticket
// against gx's real ticket validator. bundle.go embeds this directory's
// non-Go files into the gx binary.
package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

// requiredFiles are every file the bundle's relative references and the
// README's layout diagram promise exist.
var requiredFiles = []string{
	"README.md",
	"local-tracker.md",
	"gx-to-tickets/SKILL.md",
	"gx-tdd/SKILL.md",
	"gx-tdd/tests.md",
	"gx-tdd/mocking.md",
	"gx-implement/SKILL.md",
	"gx-resolving-merge-conflicts/SKILL.md",
}

func TestBundleRequiredFilesPresent(t *testing.T) {
	for _, rel := range requiredFiles {
		if _, err := os.Stat(rel); err != nil {
			t.Errorf("required bundle file %s: %v", rel, err)
		}
	}
}

// TestEmbeddedBundleContainsRequiredFiles verifies the embedded bundle
// boundary (Bundle, via BundleFiles) - not just the files on disk - carries
// every canonical runtime file, so a release/Homebrew/go-install/local build
// all ship the same content.
func TestEmbeddedBundleContainsRequiredFiles(t *testing.T) {
	got, err := BundleFiles()
	if err != nil {
		t.Fatalf("BundleFiles: %v", err)
	}
	want := append([]string{}, requiredFiles...)
	sort.Strings(want)
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("BundleFiles() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("BundleFiles() = %v, want %v", got, want)
			break
		}
	}
}

// skillFrontmatter is the subset of SKILL.md frontmatter this test cares
// about: the metadata Claude's skill picker reads, and the invocation-policy
// flag (see README.md's "Invocation policy" section).
type skillFrontmatter struct {
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	DisableModelInvocation bool   `yaml:"disable-model-invocation"`
}

// parseFrontmatter extracts and unmarshals the "---" delimited YAML block at
// the top of a skill/ticket file, mirroring tickets/schema's own frontmatter
// convention (see local-tracker.md).
func parseFrontmatter(t *testing.T, raw string) skillFrontmatter {
	t.Helper()
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		t.Fatalf("file has no opening frontmatter delimiter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		t.Fatalf("file has no closing frontmatter delimiter")
	}
	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &fm); err != nil {
		t.Fatalf("parsing frontmatter: %v", err)
	}
	return fm
}

// wantInvocationPolicy is each skill's expected disable-model-invocation
// value, per README.md's "Invocation policy" section: gx-to-tickets and
// gx-implement are explicit-invoke only (breaking work into tickets, and
// claiming/implementing one, should never trigger on the model's own reading
// of a conversation); gx-tdd and gx-resolving-merge-conflicts are left
// model-invocable.
var wantInvocationPolicy = map[string]bool{
	"gx-to-tickets":                true,
	"gx-tdd":                       false,
	"gx-implement":                 true,
	"gx-resolving-merge-conflicts": false,
}

func TestSkillMetadataAndInvocationPolicy(t *testing.T) {
	for name, wantDisabled := range wantInvocationPolicy {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(name, "SKILL.md")
			fm := parseFrontmatter(t, readFile(t, path))
			if fm.Name != name {
				t.Errorf("frontmatter name = %q, want %q", fm.Name, name)
			}
			if strings.TrimSpace(fm.Description) == "" {
				t.Errorf("frontmatter description is empty")
			}
			if fm.DisableModelInvocation != wantDisabled {
				t.Errorf("disable-model-invocation = %v, want %v", fm.DisableModelInvocation, wantDisabled)
			}
		})
	}
}

// relativeLinkRe finds markdown links to a relative .md file, e.g.
// "[local-tracker.md](../local-tracker.md)" — not an http(s) link.
var relativeLinkRe = regexp.MustCompile(`\]\(([^)]+\.md)\)`)

func TestRelativeReferencesResolve(t *testing.T) {
	bundleFiles := append([]string{}, requiredFiles...)
	for _, rel := range bundleFiles {
		raw := readFile(t, rel)
		dir := filepath.Dir(rel)
		for _, m := range relativeLinkRe.FindAllStringSubmatch(raw, -1) {
			target := m[1]
			if strings.Contains(target, "://") {
				continue
			}
			resolved := filepath.Join(dir, target)
			if _, err := os.Stat(resolved); err != nil {
				t.Errorf("%s: relative reference %q does not resolve (%s): %v", rel, target, resolved, err)
			}
		}
	}
}

// representativeTicket exercises the exact shape gx-to-tickets/SKILL.md's
// <ticket-template> produces, including an explicit "none" seam with
// rationale (the acceptance criterion for tickets that need no automated
// seam) — checked against gx's real ticket validator, not a hand-rolled
// parser, so this test fails if the template and the validator ever
// disagree about what a valid ticket looks like.
const representativeTicket = `---
id: "01"
status: ready-for-agent
blocked_by: []
split: []
type: task
expected_context_window: 20000
---
# 01 — Example ticket

## What to build

The end-to-end behaviour this ticket makes work.

## Test seams

none — this ticket only adds documentation, no automated behavior to observe.

## Acceptance criteria

- [ ] Acceptance criterion 1
`

func TestRepresentativeTicketValidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01-example.md")
	if err := os.WriteFile(path, []byte(representativeTicket), 0o644); err != nil {
		t.Fatalf("writing representative ticket: %v", err)
	}
	if _, err := schema.ParseTicket(path); err != nil {
		t.Errorf("representative ticket failed gx's validator: %v", err)
	}
}
