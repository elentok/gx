package migrations

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/tickettest"
)

func TestDefault_PaneBusyToIterationErrorRewritesPausedTicket(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "epic", "issues", "01-a.md")
	tickettest.WriteFile(t, filepath.Join(root, "epic", "ticket.md"), "---\nstatus: open\n---\n# E\n")
	tickettest.WriteFile(t, path, "---\nid: \"01\"\nstatus: paused\npause_reason: agent_pane_busy\ntype: implement\n---\nBody.\n")
	applied := filepath.Join(t.TempDir(), AppliedFile)
	env := Env{Dirs: config.DirsFrom([]string{"HOME=" + t.TempDir()}), Root: root, LookupRoot: root, AppliedPath: applied, SkipApplied: true}

	if _, _, err := Default.Run(env); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "pause_reason: iteration-error") || strings.Contains(string(raw), "agent_pane_busy") {
		t.Fatalf("pause_reason not rewritten:\n%s", raw)
	}
	tk, err := schema.ParseTicketFromRaw(string(raw), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tk.PauseReason.Row(); !ok || tk.PauseReason != schema.PauseIterationError {
		t.Errorf("reason %q has no recovery verbs (ok=%v)", tk.PauseReason, ok)
	}
	first := readLines(t, applied)
	if n := len(slices.DeleteFunc(slices.Clone(first), func(l string) bool { return l != "pane-busy-to-iteration-error" })); n != 1 {
		t.Errorf("step recorded %d times in %v, want once", n, first)
	}
	if _, _, err := Default.Run(env); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(path)
	if string(again) != string(raw) || !slices.Equal(readLines(t, applied), first) {
		t.Errorf("second run changed something:\n%s", again)
	}
}
