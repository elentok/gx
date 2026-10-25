package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// writeHomelessTranscript writes a one-turn session transcript under the test
// HOME in a project directory no worktree maps to, as a deleted worktree leaves it.
func writeHomelessTranscript(t *testing.T, id string, input int) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","timestamp":"2026-01-01T00:00:00Z","message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":` +
		strconv.Itoa(input) + `,"output_tokens":10}}}`
	writeSessionFixture(t, home, "-gone-worktree-"+id, id, []string{line})
	if matches, _ := transcript.FindByID(home, id); len(matches) != 1 {
		t.Fatalf("transcript for %s not found", id)
	}
}

func TestExecute_TicketsMigrate_BackfillsTicketTokensAndCost(t *testing.T) {
	t.Parallel()
	const prefix = "status: done\ntype: implement\n"
	id := func(name string) string { return "migrate-totals-" + t.Name() + "-" + name }
	for name, input := range map[string]int{"a1": 100, "a2": 200, "b1": 50, "p": 300, "c": 400} {
		writeHomelessTranscript(t, id(name), input)
	}

	root := t.TempDir()
	issues := writeMigrateFixtureEpic(t, root, "old", map[string]string{
		"01-two-sessions.md": "---\nid: \"01\"\n" + prefix + "session_ids: [" + id("a1") + ", " + id("a2") + "]\n---\nBody.\n",
		"02-one-gone.md":     "---\nid: \"02\"\n" + prefix + "actual_cost: 0.5\nsession_ids: [" + id("b1") + ", " + id("missing") + "]\n---\nBody.\n",
		"03-no-session.md":   "---\nid: \"03\"\n" + prefix + "claimed_at: 2026-01-01T00:00:00Z\n---\nBody.\n",
		"04-commitless.md":   "---\nid: \"04\"\n" + prefix + "commitless: true\n---\nBody.\n",
		"05-parent.md":       "---\nid: \"05\"\n" + prefix + "session_ids: [" + id("p") + "]\n---\nBody.\n",
		"05a-conflict.md":    "---\nid: \"05a\"\nstatus: done\ntype: conflict-resolution\nparent: \"05\"\n---\nBody.\n",
	})
	writeRunLog(t, filepath.Join(root, "old"), []string{
		`{"time":"2026-01-01T00:00:00Z","type":"conflict-resolved","ticket":"05","agent_session":"` + id("c") + `"}`,
	})
	testutil.WriteFile(t, filepath.Join(root, "old"), "ticket.md", "---\nstatus: open\n---\n")

	run := func() string {
		t.Helper()
		var stdout bytes.Buffer
		if err := execute([]string{"tickets", "migrate", root}, deps{stdout: &stdout, stderr: &bytes.Buffer{}}); err != nil {
			t.Fatalf("execute: %v", err)
		}
		return stdout.String()
	}
	out := run()
	if !strings.Contains(out, "skipped: unreadable transcript") || strings.Count(out, "skipped: unreadable transcript") != 1 {
		t.Errorf("want exactly one skip note, for the unreadable ticket:\n%s", out)
	}

	load := func(name string) tickets.Ticket {
		t.Helper()
		epics, err := tickets.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, tk := range epics[0].Tickets {
			if strings.HasPrefix(filepath.Base(tk.FilePath()), name) {
				return tk
			}
		}
		t.Fatalf("ticket %s not found in %s", name, issues)
		return tickets.Ticket{}
	}
	for name, want := range map[string]int64{"01": 320, "04": 0, "05-": 310, "05a": 410} {
		tk := load(name)
		if tk.TotalTokens == nil || *tk.TotalTokens != want {
			t.Errorf("ticket %s tokens = %v, want %d", name, tk.TotalTokens, want)
		}
	}
	if tk := load("01"); tk.ActualCost <= 0 {
		t.Errorf("two-session ticket cost = %v, want a summed cost", tk.ActualCost)
	}
	if tk := load("02"); tk.TotalTokens != nil || tk.ActualCost != 0.5 {
		t.Errorf("ticket with a gone transcript = %v/%v, want unstamped with its old cost", tk.TotalTokens, tk.ActualCost)
	}
	if tk := load("03"); tk.TotalTokens != nil {
		t.Errorf("claimed ticket with no sessions was stamped %v", *tk.TotalTokens)
	}

	before, err := os.ReadFile(filepath.Join(issues, "01-two-sessions.md"))
	if err != nil {
		t.Fatal(err)
	}
	run()
	after, err := os.ReadFile(filepath.Join(issues, "01-two-sessions.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("rerun rewrote the ticket:\n%s\nvs\n%s", before, after)
	}
}
