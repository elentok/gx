// Package oldwords holds the repo-wide guard for the pause terminology
// rename (docs/specs/pause-terminology.md, Seam 6): the retired words may be
// written only in the read shim, the hidden aliases, tests of those, and
// the files that document the retirement.
package oldwords

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// oldWords are the retired spellings. "land" is matched only as a token of an
// identifier, event or verb name: the project `landing` policy and prose such
// as "landed on trunk" in comments are a different concept.
var oldWords = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(^|[^a-z])(un)?(park|drain)(ed|ing|s)?([^a-z]|$)`),
	regexp.MustCompile(`[a-z](Park|Unpark|Drain)|(^|[^A-Za-z])(Park|Unpark|park|unpark|Drain|drain)[A-Z]`),
	regexp.MustCompile(`(?i)needs[-_ ]?(answer|repair)|resume-on-start`),
	regexp.MustCompile(`\bland-\w+|\w+-land\b|"land"|\bLand[A-Z]\w*|\w*[a-z]Land\b`),
}

// skillWords applies to the skill bundle: any form in plain text.
var skillWords = regexp.MustCompile(`(?i)\b(land\w*|drain\w*|park\w*|unpark\w*)`)

// allowed are the places an old word may live, each with its reason. A
// trailing slash matches a directory.
var allowed = map[string]string{
	// The read shim: the single place old logs and tickets are loaded.
	"tickets/status.go":             "read shim",
	"tickets/log.go":                "read shim",
	"tickets/migrate.go":            "read shim",
	"tickets/migrate_pause.go":      "read shim",
	"tickets/schema/ticket.go":      "read shim",
	"tickets/schema/parse.go":       "read shim",
	"tickets/schema/pause.go":       "read shim",
	"tickets/schema/frontmatter.go": "read shim",
	"events/legacy.go":              "read shim",
	// Hidden aliases.
	"config/config.go": "resume-on-start alias",
	// Documentation of the retirement and of the past.
	"docs/":                          "old specs, ADRs and plans",
	"CHANGELOG.md":                   "changelog",
	"GLOSSARY.md":                    "notes on retired terms",
	"skills/gx-investigate/SKILL.md": "old-to-new mapping",
}

func trackedFiles(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", "..", "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func isAllowed(rel string) bool {
	for p := range allowed {
		if rel == p || (strings.HasSuffix(p, "/") && strings.HasPrefix(rel, p)) {
			return true
		}
	}
	return false
}

func scanned(rel string) bool {
	switch filepath.Ext(rel) {
	case ".go", ".ts", ".tsx", ".json", ".md", ".sh", ".py", ".yaml", ".yml", ".html", ".css", ".js", "":
	default:
		return false
	}
	base := filepath.Base(rel)
	return base != "go.sum" && base != "package-lock.json" && !strings.Contains(rel, "testdata/")
}

func findOldWord(rel, line string) string {
	if strings.HasPrefix(rel, "skills/") && strings.HasSuffix(rel, ".md") {
		return skillWords.FindString(line)
	}
	return matchAny(oldWords, line)
}

func matchAny(res []*regexp.Regexp, line string) string {
	for _, re := range res {
		if m := re.FindString(line); m != "" {
			return strings.Trim(m, " .,;:()\"'`")
		}
	}
	return ""
}

func TestNoOldWords(t *testing.T) {
	for _, rel := range trackedFiles(t) {
		if !scanned(rel) || isAllowed(rel) || rel == "oldwords/oldwords_test.go" || rel == "oldwords/leftovers_test.go" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("..", rel))
		if err != nil {
			continue
		}
		found := false
		for i, line := range strings.Split(string(raw), "\n") {
			if m := findOldWord(rel, line); m != "" {
				found = true
				if !leftovers[rel] {
					t.Errorf("%s:%d: old word %q", rel, i+1, m)
				}
			}
		}
		if leftovers[rel] && !found {
			t.Errorf("%s is clean: remove it from leftovers", rel)
		}
	}
}

func TestGuardCatchesOldWords(t *testing.T) {
	for _, s := range []string{"parked", "unpark", "park_kind", "park.question", "needs-answer", "NeedsRepair",
		"resume-on-start", "draining", "manual-land", "land-conflict", "RepairLand", "parkTicket", "isParked",
		"UnparkForm", "Gate.Drain", "a park of a worktree"} {
		if matchAny(oldWords, s) == "" {
			t.Errorf("guard missed %q", s)
		}
	}
	for _, s := range []string{"paused", "merge-conflict", "landing", "on-landed", "spark", "parker", "git reset", "landscape"} {
		if m := matchAny(oldWords, s); m != "" {
			t.Errorf("guard flagged %q (%q)", s, m)
		}
	}
	if findOldWord("skills/x/SKILL.md", "it landed") == "" {
		t.Error("skill guard missed prose \"landed\"")
	}
}
