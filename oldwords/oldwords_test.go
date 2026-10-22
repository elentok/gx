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
	"slices"
	"sort"
	"strings"
	"testing"
)

// oldWords are the retired spellings. "land" is matched only as a token of an
// identifier, event or verb name: the project `landing` policy and prose such
// as "landed on trunk" in comments are a different concept. "reset" is matched
// only as the retired ticket verb, never `git reset` or the budget and
// rate-limit resets.
var oldWords = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(^|[^a-z])(un)?(park|drain)(ed|ing|s)?([^a-z]|$)`),
	regexp.MustCompile(`[a-z](Park|Unpark|Drain)|(^|[^A-Za-z])(Park|Unpark|park|unpark|Drain|drain)[A-Z]`),
	regexp.MustCompile(`(?i)needs[-_ ]?(answer|repair)|resume-on-start`),
	regexp.MustCompile(`\bland-\w+|\w+-land\b|"land"|\bLand[A-Z]\w*|\w*[a-z]Land\b`),
	regexp.MustCompile(`ticket-reset|"reset"|(repair|tickets) reset\b|\bresetDeps\b|\w*Resettable\b|\bReset(Input|Result|Deps)\b|repair\.Reset\b|\brepairReset\b|\breset verb\b`),
}

// skillWords applies to the skill bundle: any form in plain text.
var skillWords = regexp.MustCompile(`(?i)\b(land\w*|drain\w*|park\w*|unpark\w*)`)

// gitResetArgs is `git reset`'s subcommand in an argument list.
var gitResetArgs = regexp.MustCompile(`"reset",\s*"(-|HEAD)`)

// pathWords applies to tracked file paths.
var pathWords = regexp.MustCompile(`(?i)(^|[^a-z])(un)?(park|drain|land|reset)(ed|ing|s)?([^a-z]|$)|needs[-_ ]?(answer|repair)`)

// lineMarker exempts one line: `// oldword:ok <reason>`.
const lineMarker = "oldword:ok"

// families are the normalized word groups an allowance names, longest first.
var families = []string{"needsanswer", "needsrepair", "resumeonstart", "reset", "drain", "park", "land"}

type allowance struct {
	reason string
	// tokens are the families the file may use. Empty exempts the whole file,
	// which only the documentation entries below may do.
	tokens []string
}

func allow(reason string, tokens ...string) allowance { return allowance{reason, tokens} }

// allowed are the places an old word may live, each with its reason and the
// families it may use. A trailing slash matches a directory.
var allowed = map[string]allowance{
	// The read shim: the single place old logs and tickets are loaded.
	"tickets/status.go":             allow("read shim", "needsanswer", "needsrepair"),
	"tickets/log.go":                allow("read shim", "park"),
	"tickets/migrate.go":            allow("read shim", "needsanswer", "needsrepair"),
	"tickets/migrate_pause.go":      allow("read shim", "park", "needsanswer", "needsrepair"),
	"tickets/schema/ticket.go":      allow("read shim", "needsanswer", "needsrepair", "land"),
	"tickets/schema/parse.go":       allow("read shim", "needsanswer", "needsrepair"),
	"tickets/schema/pause.go":       allow("read shim", "park", "needsanswer", "needsrepair", "land"),
	"tickets/schema/frontmatter.go": allow("read shim", "park", "needsanswer", "needsrepair"),
	"events/legacy.go":              allow("read shim", "park", "needsanswer", "needsrepair", "land", "reset"),
	// Hidden aliases.
	"config/config.go":    allow("resume-on-start alias", "resumeonstart"),
	"apiclient/client.go": allow("QueueDrain alias client for the hidden drain route", "drain"),
	"cmd/server_reads.go": allow("hidden drain and park command aliases", "drain", "park"),
	"server/server.go":    allow("hidden /v1/queue/drain, /v1/tickets/park and unpark routes", "drain", "park"),
	"server/web.go":       allow("hidden /v1/tickets/unpark route", "park"),
	// Legacy section headings and statuses that old tickets still carry; the
	// code demotes, migrates or recognises them.
	"cmd/commands.go":             allow("migrate help names the legacy Needs Answer section", "needsanswer"),
	"cmd/tickets_migrate.go":      allow("migrate doc names the legacy statuses and MigrateParkSections", "park", "needsanswer", "needsrepair"),
	"ralphloop/claim.go":          allow("demotes the legacy Needs Repair section", "needsrepair"),
	"ralphloop/pauseticket.go":    allow("names the legacy Needs Repair section", "needsrepair"),
	"ralphloop/resumeticket.go":   allow("demotes the legacy Needs Answer and Needs Repair sections", "needsanswer", "needsrepair"),
	"tickets/migrate_question.go": allow("moves the legacy Needs Answer section into the log", "needsanswer"),
	"tickets/pausedetail.go":      allow("legacy Needs Answer and Needs Repair headings", "needsanswer", "needsrepair"),
	"tickets/startover.go":        allow("demotes the legacy Needs Answer and Needs Repair sections", "needsanswer", "needsrepair"),
	"ui/tickets/pause_labels.go":  allow("documents the legacy statuses it still labels", "needsanswer", "needsrepair"),
	"ui/tickets/preview.go":       allow("highlights the legacy Needs Repair heading", "needsrepair"),
	// Wire and persisted names kept for compatibility with running servers,
	// stored logs and on-disk state.
	"server/queue.go":                     allow("JSON wire fields parked, park_kind, park_reason", "park"),
	"server/epicsummary.go":               allow("JSON wire field parked", "park"),
	"server/epicdetail.go":                allow("JSON wire field needs_answer", "needsanswer"),
	"server/reads.go":                     allow("persisted lock kind \"land\"", "land"),
	"server/mergerecover.go":              allow("persisted event kind land-rolled-back", "land"),
	"server/repair.go":                    allow("hidden unpark verb and the persisted unpark answer choice", "park"),
	"cmd/server_repair.go":                allow("hidden unpark verb alias", "park"),
	"ralphloop/mergelock.go":              allow("on-disk land-locks state dir; renaming would orphan live locks", "land"),
	"ralphloop/notification_text.go":      allow("chat headlines \"needs answer\" and \"needs repair\"", "needsanswer", "needsrepair"),
	"ralphloop/waitforfinish_recovery.go": allow("comment names the legacy Needs Answer heading", "needsanswer"),
	"webui/src/generated/server.ts":       allow("generated from the wire fields above", "park", "needsanswer"),
	"webui/src/epic/EpicPage.tsx":         allow("reads the needs_answer wire field", "needsanswer"),
	"webui/src/epic/rows.ts":              allow("maps the legacy needs-answer and needs-repair statuses", "needsanswer", "needsrepair"),
	"webui/src/view.ts":                   allow("reads the legacy needs-repair status and the parked wire field", "needsrepair", "park"),
	// Tests of the read shim, the hidden aliases and the kept wire names.
	"cmd/server_parity_test.go":            allow("lists the hidden drain, park and unpark routes", "drain", "park"),
	"cmd/tickets_alias_test.go":            allow("tests the retired land, reset, unpark and drain aliases", "park", "land", "reset", "drain"),
	"cmd/tickets_migrate_test.go":          allow("migrate input uses the legacy statuses", "park", "needsanswer", "needsrepair"),
	"cmd/tickets_migrate_pause_test.go":    allow("migrate input uses the legacy statuses, park_kind and headings", "park", "land", "needsanswer", "needsrepair"),
	"cmd/tickets_migrate_question_test.go": allow("migrates the legacy Needs Answer section", "park", "needsanswer"),
	"config/agents_test.go":                allow("tests the resume-on-start alias", "resumeonstart"),
	"events/legacy_test.go":                allow("tests the legacy event reader", "park", "needsanswer", "needsrepair", "land", "reset"),
	"ralphloop/notification_text_test.go":  allow("chat headlines \"needs answer\" and \"needs repair\"", "needsanswer", "needsrepair"),
	"ralphloop/claim_test.go":              allow("tests demoting the legacy Needs Answer and Needs Repair sections", "park", "needsanswer", "needsrepair"),
	"ralphloop/resumeticket_test.go":       allow("tests demoting the legacy Needs Answer and Needs Repair sections", "needsanswer", "needsrepair"),
	"tickets/loader_test.go":               allow("loads legacy statuses, park_kind and headings", "land", "needsanswer", "needsrepair"),
	"tickets/log_test.go":                  allow("reads legacy park.* log events", "park"),
	"tickets/schema/parse_test.go":         allow("reads the legacy iteration_status needs-answer", "needsanswer"),
	"tickets/schema/pause_test.go":         allow("tests the legacy heading and kind shim", "park", "needsrepair", "land"),
	"tickets/schema/pause_write_test.go":   allow("asserts the old words are never written", "park", "needsanswer", "needsrepair", "land"),
	"webui/src/canned-snapshot.json":       allow("fixture of the kept parked wire field and legacy rendered statuses", "park", "needsanswer", "needsrepair"),
	"webui/src/canned-snapshot.test.ts":    allow("reads the kept parked wire field and legacy statuses", "park", "needsanswer", "needsrepair"),
	"webui/src/queue/QueueScreen.test.tsx": allow("builds the kept parked wire field", "park"),
	"webui/src/view.test.ts":               allow("builds the kept parked wire field", "park"),
	"webui/src/routes.test.tsx":            allow("uses the kept legacy rendered statuses and needs_answer field", "needsanswer", "needsrepair"),
	"server/epicsummary_test.go":           allow("summary reads only the legacy Needs headings", "needsanswer", "needsrepair"),
	"server/ticketwrites_test.go":          allow("unmute demotes only the legacy Needs Repair heading", "needsrepair"),
	"server/recovery_notify_test.go":       allow("snapshot status is the legacy wire word", "needsanswer", "needsrepair"),
	"server/reads_test.go":                 allow("persisted lock kind \"land\"", "land"),
	"server/web_test.go":                   allow("tests the hidden unpark route", "park"),
	"server/web_allowlist_test.go":         allow("tests the hidden unpark route", "park"),
	"server/resume_web_test.go":            allow("asserts the persisted unpark answer choice", "park"),
	"server/server_notify_test.go":         allow("asserts the \"needs answer\" and \"needs repair\" headlines", "needsanswer", "needsrepair"),
	// Documentation of the retirement and of the past.
	"docs/":                          allow("old specs, ADRs and plans"),
	"CHANGELOG.md":                   allow("changelog"),
	"GLOSSARY.md":                    allow("notes on retired terms"),
	"skills/gx-investigate/SKILL.md": allow("old-to-new mapping"),
}

func trackedFiles(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", "..", "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func allowanceFor(rel string) (string, allowance, bool) {
	for p, a := range allowed {
		if rel == p || (strings.HasSuffix(p, "/") && strings.HasPrefix(rel, p)) {
			return p, a, true
		}
	}
	return "", allowance{}, false
}

func scanned(rel string) bool {
	switch filepath.Ext(rel) {
	case ".go", ".ts", ".tsx", ".mts", ".json", ".md", ".sh", ".py", ".yaml", ".yml", ".html", ".css", ".js", ".mjs", ".cjs", "":
	default:
		return false
	}
	base := filepath.Base(rel)
	return base != "go.sum" && base != "package-lock.json" && !strings.Contains(rel, "testdata/")
}

// family names the allowance group a match belongs to.
func family(match string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(match) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	norm := b.String()
	for _, f := range families {
		if strings.Contains(norm, f) {
			return f
		}
	}
	return norm
}

// oldWordFamilies returns the family of every old word on a line.
func oldWordFamilies(rel, line string) []string {
	var res []*regexp.Regexp
	if strings.HasPrefix(rel, "skills/") && strings.HasSuffix(rel, ".md") {
		res = []*regexp.Regexp{skillWords}
	} else {
		res = oldWords
	}
	line = gitResetArgs.ReplaceAllString(line, "")
	var out []string
	for _, re := range res {
		for _, m := range re.FindAllString(line, -1) {
			out = append(out, family(m))
		}
	}
	return out
}

func findOldWord(rel, line string) string {
	if fs := oldWordFamilies(rel, line); len(fs) > 0 {
		return fs[0]
	}
	return ""
}

func matchAny(res []*regexp.Regexp, line string) string {
	for _, re := range res {
		if m := re.FindString(line); m != "" {
			return strings.Trim(m, " .,;:()\"'`")
		}
	}
	return ""
}

func (a allowance) permits(f string) bool {
	return slices.Contains(a.tokens, f)
}

func TestNoOldWords(t *testing.T) {
	used := map[string]map[string]bool{}
	for _, rel := range trackedFiles(t) {
		if !scanned(rel) || rel == "oldwords/oldwords_test.go" {
			continue
		}
		key, a, ok := allowanceFor(rel)
		if ok && len(a.tokens) == 0 {
			continue
		}
		note := func(f string) bool {
			if ok && a.permits(f) {
				if used[key] == nil {
					used[key] = map[string]bool{}
				}
				used[key][f] = true
				return true
			}
			return false
		}
		for _, m := range pathWords.FindAllString(rel, -1) {
			if f := family(m); !note(f) {
				t.Errorf("%s: old word %q in the file path", rel, f)
			}
		}
		raw, err := os.ReadFile(filepath.Join("..", rel))
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, lineMarker) {
				continue
			}
			for _, f := range oldWordFamilies(rel, line) {
				if !note(f) {
					t.Errorf("%s:%d: old word %q", rel, i+1, f)
				}
			}
		}
	}
	var keys []string
	for k := range allowed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, tok := range allowed[k].tokens {
			if !used[k][tok] {
				t.Errorf("allowed[%q]: token %q is unused; drop it (and the entry once no token is left)", k, tok)
			}
		}
	}
}

func TestGuardCatchesOldWords(t *testing.T) {
	for _, s := range []string{"parked", "unpark", "park_kind", "park.question", "needs-answer", "NeedsRepair",
		"resume-on-start", "draining", "manual-land", "land-conflict", "RepairLand", "parkTicket", "isParked",
		"UnparkForm", "Gate.Drain", "a park of a worktree", "ticket-reset", "repairReset", "ResetInput",
		"checkResettable", "repair.Reset(x)", "gx tickets repair reset"} {
		if matchAny(oldWords, s) == "" {
			t.Errorf("guard missed %q", s)
		}
	}
	for _, s := range []string{"paused", "merge-conflict", "landing", "on-landed", "spark", "parker", "git reset", "landscape",
		"budget resets daily", "keys.Reset()", "rate limit resets 3pm"} {
		if m := matchAny(oldWords, s); m != "" {
			t.Errorf("guard flagged %q (%q)", s, m)
		}
	}
	if findOldWord("skills/x/SKILL.md", "it landed") == "" {
		t.Error("skill guard missed prose \"landed\"")
	}
	if pathWords.FindString("repair/land.go") == "" || pathWords.FindString("repair/startover.go") != "" {
		t.Error("path guard mismatch")
	}
}

func TestScannedCoversScriptExtensions(t *testing.T) {
	for _, rel := range []string{"scripts/x.mjs", "scripts/x.cjs", "web/x.mts"} {
		if !scanned(rel) {
			t.Errorf("%s is not scanned", rel)
		}
	}
}

func TestAllowedFileIsExemptOnlyForItsTokens(t *testing.T) {
	a := allow("test", "park")
	if !a.permits(family("unpark")) || a.permits(family("needs-answer")) {
		t.Error("allowance should permit park and refuse needs-answer")
	}
}
