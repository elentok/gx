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
	"config/config.go":    "resume-on-start alias",
	"apiclient/client.go": "QueueDrain alias client for the hidden drain route",
	"cmd/server_reads.go": "hidden drain and park command aliases",
	"server/server.go":    "hidden /v1/queue/drain, /v1/tickets/park and unpark routes",
	"server/web.go":       "hidden /v1/tickets/unpark route",
	// Legacy section headings and statuses that old tickets still carry; the
	// code demotes, migrates or recognises them.
	"cmd/commands.go":             "migrate help names the legacy Needs Answer section",
	"cmd/tickets_migrate.go":      "migrate doc names the legacy statuses",
	"ralphloop/claim.go":          "demotes the legacy Needs Repair section",
	"ralphloop/pauseticket.go":    "names the legacy Needs Repair section",
	"ralphloop/resumeticket.go":   "demotes the legacy Needs Answer and Needs Repair sections",
	"tickets/migrate_question.go": "moves the legacy Needs Answer section into the log",
	"tickets/pausedetail.go":      "legacy Needs Answer and Needs Repair headings",
	"tickets/startover.go":        "demotes the legacy Needs Answer and Needs Repair sections",
	"ui/tickets/pause_labels.go":  "documents the legacy statuses it still labels",
	"ui/tickets/preview.go":       "highlights the legacy Needs Repair heading",
	// Wire and persisted names kept for compatibility with running servers,
	// stored logs and on-disk state.
	"server/queue.go":                     "JSON wire fields parked, park_kind, park_reason",
	"server/epicsummary.go":               "JSON wire field parked",
	"server/epicdetail.go":                "JSON wire field needs_answer",
	"server/reads.go":                     "persisted lock kind \"land\"",
	"server/mergerecover.go":              "persisted event kind land-rolled-back",
	"server/repair.go":                    "hidden unpark verb and the persisted unpark answer choice",
	"cmd/server_repair.go":                "hidden unpark verb alias",
	"ralphloop/mergelock.go":              "on-disk land-locks state dir; renaming would orphan live locks",
	"ralphloop/notification_text.go":      "chat headlines \"needs answer\" and \"needs repair\"",
	"ralphloop/waitforfinish_recovery.go": "comment names the legacy Needs Answer heading",
	"webui/src/generated/server.ts":       "generated from the wire fields above",
	"webui/src/epic/EpicPage.tsx":         "reads the needs_answer wire field",
	"webui/src/epic/rows.ts":              "maps the legacy needs-answer and needs-repair statuses",
	"webui/src/view.ts":                   "reads the legacy needs-repair status and the parked wire field",
	// Tests of the read shim, the hidden aliases and the kept wire names.
	"cmd/server_parity_test.go":            "lists the hidden drain, park and unpark routes",
	"cmd/tickets_alias_test.go":            "tests the hidden park and unpark aliases",
	"cmd/tickets_migrate_test.go":          "migrate input uses the legacy statuses",
	"cmd/tickets_migrate_pause_test.go":    "migrate input uses the legacy statuses, park_kind and headings",
	"cmd/tickets_migrate_park_test.go":     "migrates the legacy Needs Answer section",
	"config/agents_test.go":                "tests the resume-on-start alias",
	"events/legacy_test.go":                "tests the legacy event reader",
	"ralphloop/notification_text_test.go":  "chat headlines \"needs answer\" and \"needs repair\"",
	"ralphloop/claim_test.go":              "tests demoting the legacy Needs Repair section",
	"ralphloop/resumeticket_test.go":       "tests demoting the legacy Needs Answer and Needs Repair sections",
	"tickets/loader_test.go":               "loads legacy statuses, park_kind and headings",
	"tickets/log_test.go":                  "reads legacy park.* log events",
	"tickets/schema/parse_test.go":         "reads the legacy iteration_status needs-answer",
	"tickets/schema/pause_test.go":         "tests the legacy heading and kind shim",
	"tickets/schema/pause_write_test.go":   "asserts the old words are never written",
	"webui/src/canned-snapshot.json":       "fixture of the kept parked wire field and legacy rendered statuses",
	"webui/src/canned-snapshot.test.ts":    "reads the kept parked wire field",
	"webui/src/queue/QueueScreen.test.tsx": "builds the kept parked wire field",
	"webui/src/view.test.ts":               "builds the kept parked wire field",
	"webui/src/routes.test.tsx":            "uses the kept legacy rendered statuses and needs_answer field",
	"server/epicsummary_test.go":           "summary reads only the legacy Needs headings",
	"server/ticketwrites_test.go":          "unmute demotes only the legacy Needs Repair heading",
	"server/recovery_notify_test.go":       "snapshot status is the legacy wire word",
	"server/reads_test.go":                 "persisted lock kind \"land\"",
	"server/web_test.go":                   "tests the hidden unpark route",
	"server/web_allowlist_test.go":         "tests the hidden unpark route",
	"server/resume_web_test.go":            "asserts the persisted unpark answer choice",
	"server/server_notify_test.go":         "asserts the \"needs answer\" and \"needs repair\" headlines",
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
		if !scanned(rel) || isAllowed(rel) || rel == "oldwords/oldwords_test.go" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("..", rel))
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if m := findOldWord(rel, line); m != "" {
				t.Errorf("%s:%d: old word %q", rel, i+1, m)
			}
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
