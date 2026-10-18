package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/tickets"
)

// serverLandFixture is a claimed ticket built and waiting to land through the
// server's landBuilt.
func serverLandFixture(t *testing.T) (Deps, iterationParams, *builtAwaitingLandError, string) {
	t.Helper()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	tk := epics[0].Tickets[0]
	d, _, _ := fakeDeps()
	p := iterationParams{
		WorkspaceID: "ws1", RepoDir: "/fake/repo", WorktreeDir: "/fake/worktrees", FeatureWorktree: "/fake/feature",
		FeatureBranch: "epic", Agent: AgentClaude, Skill: "implement", Ticket: tk, ScratchDir: scratchDir,
		WorktreeLock: &sync.Mutex{}, SmartZone: defaultSmartZone, Gate: NewGate(), Sink: noopEventSink{},
	}
	built := &builtAwaitingLandError{job: landJob{
		ticket: tk, base: "base", branch: iterBranch("epic", "01"), path: iterationWorktreePath("/fake/worktrees", "epic", "01"),
	}}
	return d, p, built, filepath.Join(scratchDir, "epic", "issues", "01-a.md")
}

// A conflict the resolution child cannot clear must park the parent with a
// cause, not leave it claimed with nobody told.
func TestLandBuilt_UnresolvedConflictParksParentAsLandConflict(t *testing.T) {
	t.Parallel()
	d, p, built, ticketPath := serverLandFixture(t)
	picked := false
	d.CherryPickRange = func(string, string, string) error { picked = true; return &fakeConflictErr{} }
	// The resolver reports done but the sequencer stays conflicted.
	d.CherryPickInProgress = func(string) (bool, error) { return picked, nil }

	if err := landBuilt(d, p, built); err != nil {
		t.Fatalf("landBuilt() error = %v", err)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{"status: needs-repair", "park_kind: land-conflict", "01a"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("parent ticket missing %q:\n%s", want, raw)
		}
	}
}

// The server's finish sink tells chat a conflict resolution started and that
// its child parked, but leaves the parent's own park to the server.
func TestLandBuilt_FinishSinkSendsConflictStartAndChildParkOnly(t *testing.T) {
	t.Parallel()
	d, p, built, ticketPath := serverLandFixture(t)
	picked := false
	d.CherryPickRange = func(string, string, string) error { picked = true; return &fakeConflictErr{} }
	d.CherryPickInProgress = func(string) (bool, error) { return picked, nil }
	srv, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})
	p.Sink = c.FinishSink("alpha", nil, "epic", ticketPath, "01", func() EpicCounts { return EpicCounts{Total: 2} })

	if err := landBuilt(d, p, built); err != nil {
		t.Fatalf("landBuilt() error = %v", err)
	}
	c.Close()

	got := reqs()
	if len(got) != 1 {
		t.Fatalf("got %d sends, want one batch", len(got))
	}
	text := got[0].Text
	if !strings.Contains(text, "resolving land conflict") {
		t.Errorf("missing the conflict-started message:\n%s", text)
	}
	if n := strings.Count(text, "needs repair"); n != 1 || !strings.Contains(text, "epic/01a") {
		t.Errorf("want exactly the child's park (01a), got %d needs-repair messages:\n%s", n, text)
	}
	if strings.Contains(text, "land conflict not resolved") {
		t.Errorf("parent's own park reached chat; the server already notifies it:\n%s", text)
	}
}

// A cherry-pick in progress that no iteration branch holds is a person's work:
// the land refuses instead of aborting it.
func TestLandBuilt_ForeignCherryPickIsNotAborted(t *testing.T) {
	t.Parallel()
	d, p, built, _ := serverLandFixture(t)
	d.CherryPickInProgress = func(string) (bool, error) { return true, nil }
	d.IsAncestor = func(string, string, string) (bool, error) { return false, nil }
	aborted := false
	d.AbortCherryPick = func(string) error { aborted = true; return nil }

	err := landBuilt(d, p, built)
	if err == nil || !strings.Contains(err.Error(), "gx did not start") {
		t.Fatalf("landBuilt() error = %v, want a refusal naming the foreign cherry-pick", err)
	}
	if aborted {
		t.Error("aborted a cherry-pick gx did not start")
	}
}
