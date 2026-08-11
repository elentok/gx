package ralphloop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/herdr"
)

// stuckCompactionDeps returns fakeDeps wired so every poll tick times out with
// occupancy still over the smart zone, and every "/compact" the recovery
// submits is claimed complete by the pane but never corroborated by a
// transcript boundary — the gated give-up waitForFinish bounds and escalates
// on. onBreach runs when the breach's interrupting Ctrl-C is sent, so a caller
// can fail the iteration there instead to model an ordinary, non-compaction
// error at the same point in the loop.
func stuckCompactionDeps(onBreach func() error) Deps {
	d, _, _ := fakeDeps()
	// A native session id is what makes occupancy readable at all, so without
	// one the poll loop would never classify a breach.
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "session-01"}, nil
	}
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		// The launch's own unbounded wait succeeds, and so does every wait on
		// the compaction's states — that premature idle report is the pane
		// claiming a completion the transcript never records. The iteration's
		// own bounded poll ticks time out, which is what keeps driving the
		// breach handling.
		if opts.TimeoutMs == 0 || slices.Contains(opts.Until, "blocked") {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		}
		return herdr.Agent{}, errors.New("timed out waiting for agent status")
	}
	d.AgentSendKeys = func(string, ...string) error { return onBreach() }
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) { return 200, true, nil }
	d.ReadCompactions = func(cwd, sessionID string) (int, bool, error) { return 0, true, nil }
	d.AgentRead = func(string, herdr.AgentReadOptions) (string, error) { return "compaction complete", nil }
	return d
}

// readTicket returns the on-disk contents of the single-ticket fixture epic
// these tests share.
func readTicket(t *testing.T, scratchDir, epicName, filename string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(scratchDir, epicName, "issues", filename))
	if err != nil {
		t.Fatalf("ReadFile ticket: %v", err)
	}
	return string(raw)
}

// TestRun_UnconfirmedCompactionEscalation_PersistsNeedsRepair asserts the
// half of the escalation a waitForFinish unit test structurally cannot: that
// the error it returns is carried by Run's per-result handling all the way to
// the ticket file, as needs-repair with a reason an operator can act on.
// Without this the escalation would only end the iteration, and a stuck agent
// would look like a run that quietly stopped.
func TestRun_UnconfirmedCompactionEscalation_PersistsNeedsRepair(t *testing.T) {
	const epicName = "my-epic"
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d := stuckCompactionDeps(func() error { return nil })

	var out bytes.Buffer
	// The escalated iteration leaves the epic's only ticket needs-repair, so
	// the run parks on it rather than returning.
	runUntilParked(t, RunOptions{
		EpicName: epicName, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: "/fake/repo", SmartZone: 100,
	}, d, NewTextEventSink(&out))

	contents := readTicket(t, scratchDir, epicName, "01-first.md")
	if !strings.Contains(contents, "status: needs-repair") {
		t.Errorf("ticket after escalation =\n%s\nwant status: needs-repair", contents)
	}
	for _, unwanted := range []string{"status: done", "status: needs-answer"} {
		if strings.Contains(contents, unwanted) {
			t.Errorf("ticket after escalation =\n%s\nmust not contain %q", contents, unwanted)
		}
	}
	if !strings.Contains(contents, errCompactRecoveryExhausted.Error()) ||
		!strings.Contains(contents, errCompactNeverConfirmed.Error()) {
		t.Errorf("ticket after escalation =\n%s\nwant a reason naming the unconfirmed compaction, not a generic recovery failure", contents)
	}
}

// TestRun_OrdinaryIterationError_KeepsItsOwnNeedsRepairReason pins the
// discrimination: the escalation reason is specific to a compaction that never
// confirmed, so an unrelated failure at the same point in the loop must not
// acquire it.
func TestRun_OrdinaryIterationError_KeepsItsOwnNeedsRepairReason(t *testing.T) {
	const epicName = "my-epic"
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d := stuckCompactionDeps(func() error { return errors.New("herdr pane vanished") })

	var out bytes.Buffer
	runUntilParked(t, RunOptions{
		EpicName: epicName, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: "/fake/repo", SmartZone: 100,
	}, d, NewTextEventSink(&out))

	contents := readTicket(t, scratchDir, epicName, "01-first.md")
	if !strings.Contains(contents, "status: needs-repair") ||
		!strings.Contains(contents, "herdr pane vanished") {
		t.Errorf("ticket after ordinary failure =\n%s\nwant needs-repair carrying the failure's own reason", contents)
	}
	if strings.Contains(contents, errCompactRecoveryExhausted.Error()) ||
		strings.Contains(contents, errCompactNeverConfirmed.Error()) {
		t.Errorf("ticket after ordinary failure =\n%s\nmust not claim an unconfirmed compaction", contents)
	}
}
