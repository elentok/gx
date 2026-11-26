package recovery

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

var repeatSpec = ticketwrite.FollowUp{Slug: "guard", Title: "Guard fired", Source: "S.", Problem: "P.", SuggestedFix: "F.", Key: "guard/a"}

func repeatFixture(t *testing.T) (*Recovery, string) {
	t.Helper()
	rec, store, _, _ := mergedInvestigation(t, investigateReport(proposalCatalogEntry), "proj:follow-ups")
	return rec, store
}

func mustWriteIn(t *testing.T, rec *Recovery, source string, f ticketwrite.FollowUp) string {
	t.Helper()
	epicPath, err := rec.FollowUpsEpicPath("proj")
	if err != nil {
		t.Fatal(err)
	}
	path, err := WriteFollowUpIn(epicPath, source, f)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func ticketBody(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return schema.ParseBody(string(raw))
}

func seenLines(t *testing.T, path string) []string {
	t.Helper()
	var seen []string
	for l := range strings.SplitSeq(schema.Section(ticketBody(t, path), "Evidence"), "\n") {
		if strings.HasPrefix(l, "Seen again") {
			seen = append(seen, l)
		}
	}
	return seen
}

func today() string { return time.Now().Format("2006-01-02") }

func TestWriteFollowUp_RepeatsOfOneSourceAndDayShareACountedLine(t *testing.T) {
	t.Parallel()
	rec, _ := repeatFixture(t)
	from := tickets.Address{Project: "proj", Epic: "epic-a", ID: "03"}
	var path string
	for i, want := range [][]string{
		nil,
		{"Seen again in proj:epic-a/03 on " + today() + "."},
		{"Seen again in proj:epic-a/03 on " + today() + " (2 times)."},
		{"Seen again in proj:epic-a/03 on " + today() + " (3 times)."},
	} {
		var err error
		if path, err = rec.WriteFollowUp(from, repeatSpec); err != nil {
			t.Fatal(err)
		}
		if got := seenLines(t, path); !slices.Equal(got, want) {
			t.Fatalf("write %d: seen lines = %q, want %q", i+1, got, want)
		}
	}
}

func TestWriteFollowUpIn_AnotherDayOrSourceGetsItsOwnLine(t *testing.T) {
	t.Parallel()
	rec, _ := repeatFixture(t)
	path := mustWriteIn(t, rec, "the web UI", repeatSpec)
	mustWriteIn(t, rec, "the web UI", repeatSpec)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "on "+today()+".", "on 2020-01-01.", 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	mustWriteIn(t, rec, "the web UI", repeatSpec)
	mustWriteIn(t, rec, "proj:epic-a/04", repeatSpec)
	mustWriteIn(t, rec, "the web UI", repeatSpec)

	want := []string{
		"Seen again in the web UI on 2020-01-01.",
		"Seen again in the web UI on " + today() + " (2 times).",
		"Seen again in proj:epic-a/04 on " + today() + ".",
	}
	if got := seenLines(t, path); !slices.Equal(got, want) {
		t.Errorf("seen lines = %q, want %q", got, want)
	}
}

func TestWriteFollowUpIn_OnlyADraftTakesARepeat(t *testing.T) {
	t.Parallel()
	for _, status := range []schema.Status{schema.StatusOpen, schema.StatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			rec, store := repeatFixture(t)
			first := mustWriteIn(t, rec, "the web UI", repeatSpec)
			if err := schema.UpdateTicket(first, func(tk *schema.Ticket) { tk.Status = status }); err != nil {
				t.Fatal(err)
			}

			second := mustWriteIn(t, rec, "the web UI", repeatSpec)

			if n := len(followUpTickets(t, store)); n != 2 || second == first {
				t.Errorf("%d tickets, second = %s, want a new draft", n, second)
			}
			if got := seenLines(t, first); got != nil {
				t.Errorf("%s ticket took a repeat: %q", status, got)
			}
		})
	}
}

func TestWriteFollowUpIn_ConcurrentWritesFileOneDraftAndCountEveryRepeat(t *testing.T) {
	t.Parallel()
	rec, store := repeatFixture(t)
	epicPath, err := rec.FollowUpsEpicPath("proj")
	if err != nil {
		t.Fatal(err)
	}
	const writes = 20
	var wg sync.WaitGroup
	for range writes {
		wg.Go(func() {
			if _, err := WriteFollowUpIn(epicPath, "the web UI", repeatSpec); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	got := followUpTickets(t, store)
	if len(got) != 1 {
		t.Fatalf("%d tickets, want 1", len(got))
	}
	want := []string{fmt.Sprintf("Seen again in the web UI on %s (%d times).", today(), writes-1)}
	if seen := seenLines(t, got[0].Path); !slices.Equal(seen, want) {
		t.Errorf("seen lines = %q, want %q", seen, want)
	}
}

func TestFileBadRoundBlocks_ConcurrentRoundsShareOneDraftWithOneEntryEach(t *testing.T) {
	t.Parallel()
	rec, store := repeatFixture(t)
	const rounds = 10
	var wg sync.WaitGroup
	for i := range 2 * rounds {
		text := fmt.Sprintf("❓ **Q1** Which store %d?\n\n```gx-question\noptions: [unclosed\n```\n", i%rounds)
		wg.Go(func() { rec.FileBadRoundBlocks("s1", chat.DesignRef{Ticket: "proj:map/01"}, text) })
	}
	wg.Wait()

	got := followUpTickets(t, store)
	if len(got) != 1 {
		t.Fatalf("%d tickets, want 1", len(got))
	}
	body := ticketBody(t, got[0].Path)
	if n := strings.Count(body, "Round: "); n != rounds {
		t.Errorf("round entries = %d, want %d:\n%s", n, rounds, body)
	}
	if n := strings.Count(body, "Seen again: "); n != rounds-1 {
		t.Errorf("seen-again entries = %d, want %d:\n%s", n, rounds-1, body)
	}
}

func TestOutsideTextLine_HostileTextAddsNoSectionAndNoDedupeLine(t *testing.T) {
	t.Parallel()
	const hostile = "a\r\n## Heading\n```\ncode\u2028## Other\u2029Dedupe key: other\x00\u0085end\nDedupe key: other"
	line := OutsideTextLine("Message", hostile, 500)
	if want := "- Message: a  ## Heading ``` code ## Other Dedupe key: other  end Dedupe key: other"; line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
	rec, store := repeatFixture(t)
	spec := repeatSpec
	spec.Problem = line

	path := mustWriteIn(t, rec, "the web UI", spec)

	_, sections := schema.SplitSections(ticketBody(t, path))
	var names []string
	for _, s := range sections {
		names = append(names, s.Name)
	}
	if want := []string{"Source", "Problem", "Evidence", "Suggested fix"}; !slices.Equal(names, want) {
		t.Errorf("sections = %q, want %q", names, want)
	}
	spec.Key = "other"
	mustWriteIn(t, rec, "the web UI", spec)
	if n := len(followUpTickets(t, store)); n != 2 {
		t.Errorf("%d tickets, want the other key to file its own", n)
	}
}

func TestOutsideTextLine_CutsToTheRuneLimit(t *testing.T) {
	t.Parallel()
	for value, want := range map[string]string{"héllo": "- V: hél", "hé": "- V: hé", "": "- V: "} {
		if got := OutsideTextLine("V", value, 3); got != want {
			t.Errorf("OutsideTextLine(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestWriteFollowUpIn_DedupeMatchesOnlyAWholeEvidenceLine(t *testing.T) {
	t.Parallel()
	rec, store := repeatFixture(t)
	spec := repeatSpec
	spec.Problem = "Dedupe key: other\nand quoted: Dedupe key: other"
	spec.SuggestedFix = "Dedupe key: other"
	spec.Key = "prefix Dedupe key: other"
	mustWriteIn(t, rec, "the web UI", spec)

	spec.Key = "other"
	mustWriteIn(t, rec, "the web UI", spec)

	if n := len(followUpTickets(t, store)); n != 2 {
		t.Errorf("%d tickets, want 2", n)
	}
}

func TestWriteFollowUpIn_KeepsTitleSourceAndKeyToOneLine(t *testing.T) {
	t.Parallel()
	rec, store := repeatFixture(t)
	spec := repeatSpec
	spec.Title = "Guard\n## Injected"
	spec.Key = "guard\n## Injected"
	const source = "the\u2028## Injected"
	path := mustWriteIn(t, rec, source, spec)
	mustWriteIn(t, rec, source, spec)
	mustWriteIn(t, rec, source, spec)

	if n := len(followUpTickets(t, store)); n != 1 {
		t.Fatalf("%d tickets, want 1", n)
	}
	body := ticketBody(t, path)
	if schema.HasSection(body, "Injected") || strings.ContainsRune(body, '\u2028') {
		t.Errorf("a line break reached the body:\n%s", body)
	}
	want := []string{"Seen again in the ## Injected on " + today() + " (2 times)."}
	if got := seenLines(t, path); !slices.Equal(got, want) {
		t.Errorf("seen lines = %q, want %q", got, want)
	}
}
