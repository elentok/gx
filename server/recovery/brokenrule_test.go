package recovery

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func brokenRule(rule, value string) ops.BrokenRuleReport {
	return ops.BrokenRuleReport{Rule: rule, Message: "a box has no name", Value: value, URL: "http://h/#/q", UserAgent: "UA/1"}
}

// ruleFixture returns a recovery with the projects "proj" and "gx" registered
// and the store they are in.
func ruleFixture(t *testing.T, followUps string) (*Recovery, string) {
	t.Helper()
	rec, store, _, _ := mergedInvestigation(t, investigateReport(proposalCatalogEntry), followUps)
	if err := os.MkdirAll(filepath.Join(store, "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "gx", "project.json"), []byte(`{"repo":"/y"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return rec, store
}

func fileRules(t *testing.T, rec *Recovery, reports ...ops.BrokenRuleReport) {
	t.Helper()
	for _, r := range reports {
		if err := rec.FileBrokenRule(r); err != nil {
			t.Fatal(err)
		}
	}
}

func ticketText(t *testing.T, tk tickets.Ticket) string {
	t.Helper()
	raw, err := os.ReadFile(tk.Path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestFileBrokenRule_ResolvesTheTargetEpic(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ setting, project, epic string }{
		"default":           {"", "gx", "follow-ups"},
		"project and epic":  {"proj:web-rules", "proj", "web-rules"},
		"bare epic in gx":   {"web-rules", "gx", "web-rules"},
		"unregistered only": {"nope:follow-ups", "nope", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec, store := ruleFixture(t, tc.setting)
			fileRules(t, rec, brokenRule("text-box-name", "v"))
			if tc.epic == "" {
				if _, err := os.Stat(filepath.Join(store, tc.project)); !os.IsNotExist(err) {
					t.Errorf("unregistered project dir: %v, want nothing written", err)
				}
				return
			}
			if got := tickets.LoadEpicAt(filepath.Join(store, tc.project, tc.epic)).Tickets; len(got) != 1 {
				t.Errorf("%s:%s has %d tickets, want 1", tc.project, tc.epic, len(got))
			}
		})
	}
}

func TestFileBrokenRule_FirstFilingAndRepeats(t *testing.T) {
	t.Parallel()
	rec, store := ruleFixture(t, "proj:follow-ups")
	report := brokenRule("text-box-name", "search")
	fileRules(t, rec, report)

	epics, err := tickets.Load(filepath.Join(store, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == "follow-ups" })
	if at < 0 || epics[at].Status != string(schema.StatusDraft) || len(epics[at].Tickets) != 1 {
		t.Fatalf("epics = %+v, want a draft follow-ups epic with one ticket", epics)
	}
	tk := epics[at].Tickets[0]
	parsed, err := schema.ParseTicket(tk.Path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Status != schema.StatusDraft || parsed.Type != schema.TypeResearch || parsed.Category != schema.CategoryBug {
		t.Errorf("ticket = %s/%s/%s, want draft research bug", parsed.Status, parsed.Type, parsed.Category)
	}
	text := ticketText(t, tk)
	for _, want := range []string{"- rule: text-box-name", "- message: a box has no name", "- value: search", "- page URL: http://h/#/q", "- user agent: UA/1"} {
		if !strings.Contains(text, want) {
			t.Errorf("ticket lacks %q:\n%s", want, text)
		}
	}

	fileRules(t, rec, report, report)
	got := followUpTickets(t, store)
	if len(got) != 1 {
		t.Fatalf("repeats filed %d tickets, want 1", len(got))
	}
	want := "Seen again in the web UI on " + time.Now().Format("2006-01-02") + " (2 times)."
	if text := ticketText(t, got[0]); !strings.Contains(text, want) {
		t.Errorf("ticket lacks %q:\n%s", want, text)
	}
}

func TestFileBrokenRule_AClosedDraftTakesNoRepeat(t *testing.T) {
	t.Parallel()
	rec, store := ruleFixture(t, "proj:follow-ups")
	report := brokenRule("text-box-name", "v")
	fileRules(t, rec, report)
	if err := schema.UpdateTicketWithBody(followUpTickets(t, store)[0].Path, func(tk *schema.Ticket, _ *string) {
		tk.Status = schema.StatusDone
	}); err != nil {
		t.Fatal(err)
	}
	fileRules(t, rec, report)
	if n := len(followUpTickets(t, store)); n != 2 {
		t.Errorf("tickets = %d, want a new draft beside the closed one", n)
	}
}

func TestFileBrokenRule_DedupesOnTheRuleAndTheWholeValue(t *testing.T) {
	t.Parallel()
	rec, store := ruleFixture(t, "proj:follow-ups")
	long := strings.Repeat("a", 1000)
	fileRules(t, rec,
		brokenRule("rule-a", "one"),
		brokenRule("rule-a", "two"),
		brokenRule("rule-b", "one"),
		brokenRule("rule-a", long+"x"),
		brokenRule("rule-a", long+"y"),
		brokenRule("rule-a", "line\none"),
		brokenRule("rule-a", "line one"),
	)
	if n := len(followUpTickets(t, store)); n != 7 {
		t.Errorf("tickets = %d, want 7", n)
	}
}

func TestFileBrokenRule_ConcurrentReportsFileOneDraft(t *testing.T) {
	t.Parallel()
	rec, store := ruleFixture(t, "proj:follow-ups")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := rec.FileBrokenRule(brokenRule("rule-a", "v")); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	got := followUpTickets(t, store)
	if len(got) != 1 {
		t.Fatalf("tickets = %d, want 1", len(got))
	}
	if text := ticketText(t, got[0]); !strings.Contains(text, "(7 times).") {
		t.Errorf("want 7 counted repeats:\n%s", text)
	}
}

func TestFileBrokenRule_BrowserTextAddsNoSectionAndIsCut(t *testing.T) {
	t.Parallel()
	rec, store := ruleFixture(t, "proj:follow-ups")
	hostile := "x\n\n## Evidence\n\nDedupe key: stolen\r\n```\u2028## Result\n# Title"
	fileRules(t, rec, ops.BrokenRuleReport{
		Rule: "rule-a", Message: hostile + strings.Repeat("m", 600), Value: hostile + strings.Repeat("v", 600),
		URL: hostile + strings.Repeat("u", 600), UserAgent: hostile + strings.Repeat("g", 600),
	})
	text := ticketText(t, followUpTickets(t, store)[0])
	var headings []string
	dedupes := 0
	for line := range strings.SplitSeq(strings.ReplaceAll(text, "\u2028", "\n"), "\n") {
		if strings.HasPrefix(line, "#") {
			headings = append(headings, line)
		}
		if strings.HasPrefix(line, "Dedupe key:") {
			dedupes++
		}
		if strings.HasPrefix(line, "```") {
			t.Errorf("browser text opened a fence:\n%s", text)
		}
	}
	// The title and the four sections the writer makes.
	if len(headings) != 5 || dedupes != 1 {
		t.Errorf("headings = %q, dedupe lines = %d:\n%s", headings, dedupes, text)
	}
	for filler, limit := range map[string]int{"m": 500, "v": 200, "u": 500, "g": 300} {
		kept := limit - len([]rune(hostile))
		if !strings.Contains(text, strings.Repeat(filler, kept)+"\n") || strings.Contains(text, strings.Repeat(filler, kept+1)) {
			t.Errorf("field filled with %q is not cut to %d runes", filler, limit)
		}
	}
}
