// Package skills holds gx's canonical, installable skill bundle (see
// gx.md) and validates it: metadata, invocation policy, relative
// references between bundle files, and a representative generated ticket
// against gx's real ticket validator. bundle.go embeds this directory's
// non-Go files into the gx binary.
package skills

import (
	"io/fs"
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
	"gx.md",
	"gx-local-tracker.md",
	"gx-to-tickets/SKILL.md",
	"gx-tdd/SKILL.md",
	"gx-tdd/tests.md",
	"gx-tdd/mocking.md",
	"gx-implement/SKILL.md",
	"gx-resolving-merge-conflicts/SKILL.md",
	"gx-investigate/SKILL.md",
	"gx-investigate/gotchas.md",
	"gx-cleanup/SKILL.md",
	"gx-merge/SKILL.md",
	"gx-code-review/SKILL.md",
	"gx-changelog/SKILL.md",
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

// TestEmbeddedBundleIncludesEverySkillDir guards bundle.go's //go:embed
// directive directly against the skills/ directory on disk, rather than
// against requiredFiles (a hand-maintained list that can itself go stale -
// exactly what let gx-code-review ship installable on disk but absent from
// Bundle, since //go:embed silently omits any path not named in the
// directive).
func TestEmbeddedBundleIncludesEverySkillDir(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading skills/: %v", err)
	}
	got, err := BundleFiles()
	if err != nil {
		t.Fatalf("BundleFiles: %v", err)
	}
	embedded := make(map[string]bool)
	for _, f := range got {
		embedded[strings.SplitN(f, "/", 2)[0]] = true
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !embedded[e.Name()] {
			t.Errorf("skill directory %q exists on disk but isn't embedded in Bundle; add it to bundle.go's //go:embed directive", e.Name())
		}
	}
}

// skillFrontmatter is the subset of SKILL.md frontmatter this test cares
// about: the metadata Claude's skill picker reads, and the invocation-policy
// flag (see gx.md's "Invocation policy" section).
type skillFrontmatter struct {
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	DisableModelInvocation bool   `yaml:"disable-model-invocation"`
}

// parseFrontmatter extracts and unmarshals the "---" delimited YAML block at
// the top of a skill/ticket file, mirroring tickets/schema's own frontmatter
// convention (see gx-local-tracker.md).
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
// value, per gx.md's "Invocation policy" section: gx-implement is
// explicit-invoke only (claiming/implementing a ticket should never trigger
// on the model's own reading of a conversation); gx-to-tickets, gx-tdd,
// gx-resolving-merge-conflicts, and gx-investigate are left model-invocable.
var wantInvocationPolicy = map[string]bool{
	"gx-to-tickets":                false,
	"gx-tdd":                       false,
	"gx-implement":                 true,
	"gx-resolving-merge-conflicts": false,
	"gx-investigate":               false,
	"gx-cleanup":                   true,
	"gx-merge":                     true,
	"gx-code-review":               true,
	"gx-changelog":                 true,
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
// "[gx-local-tracker.md](../gx-local-tracker.md)" — not an http(s) link.
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

// retiredTrackerTerms are the tracker vocabulary the lifecycle contract and
// the needs-answer/needs-repair rename removed: the `children` frontmatter
// field (fork descendants are derived from `parent` alone), the statuses the
// loader no longer accepts, and `isHumanClearable`'s old name (now
// `isParked`). A skill file naming any of them hands an agent an instruction
// that produces a ticket gx itself rejects, so the bundle bans them
// outright — including in prose, where a historical mention has to be
// reworded rather than quoted verbatim.
//
// "agent_status:" is the narrowed form of the field-name ban: a bare
// substring ban on "agent_status" would break the docs whose job is to
// describe herdr's own pane-status field of that name. Requiring the
// trailing colon catches the frontmatter-style mistake (`agent_status:` as a
// YAML key) while leaving `` `agent_status` `` in prose and quoted JSON
// payloads (`"agent_status": "blocked"`, where the colon follows a quote,
// not the bare word) writable.
var retiredTrackerTerms = []string{
	"--children",
	"children:",
	"needs-triage",
	"ready-for-agent",
	"ready-for-human",
	"needs-info",
	"needs-attention",
	"isHumanClearable",
	"agent_status:",
}

func TestBundleDropsRetiredTrackerTerms(t *testing.T) {
	for _, rel := range bundleMarkdownFiles(t) {
		raw := readFile(t, rel)
		for _, term := range retiredTrackerTerms {
			if strings.Contains(raw, term) {
				t.Errorf("%s mentions retired tracker term %q", rel, term)
			}
		}
	}
}

// bundleMarkdownFiles is every markdown file in the bundle directory, not
// just requiredFiles — a skill that isn't a required runtime file still
// instructs an agent.
func bundleMarkdownFiles(t *testing.T) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking bundle: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no markdown files found in bundle")
	}
	return found
}

// representativeTicket exercises the exact shape gx-to-tickets/SKILL.md's
// <ticket-template> produces, including an explicit "none" seam with
// rationale (the acceptance criterion for tickets that need no automated
// seam) — checked against gx's real ticket validator, not a hand-rolled
// parser, so this test fails if the template and the validator ever
// disagree about what a valid ticket looks like.
const representativeTicket = `---
id: "01"
status: open
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
