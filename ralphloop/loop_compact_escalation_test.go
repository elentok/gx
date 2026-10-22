package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/schema"
)

// stuckCompactionDeps returns fakeDeps wired so every poll tick times out with
// occupancy still over the smart zone, and every "/compact" the recovery
// submits is claimed complete by the pane but never corroborated by a
// transcript boundary — the gated give-up waitForFinish bounds and escalates
// on. A non-nil interruptErr fails the breach's interrupt instead, to model an
// ordinary, non-compaction error at the same point in the loop.
func stuckCompactionDeps(interruptErr error) Deps {
	d, _, _ := fakeDeps()
	// A native session id is what makes occupancy readable at all, so without
	// one the poll loop would never classify a breach.
	// The main poll's every wait times out, which is what keeps re-driving the
	// breach; the compaction wait sees fakeDeps' agent done right after
	// "/compact", a premature idle ReadCompactions never corroborates.
	d.Runner = &blipRunner{Runner: fakeRunner(d), timeoutIf: alwaysTimeout, finishPollsOnly: true, interruptErr: interruptErr}
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) { return 200, true, nil }
	d.ReadCompactions = func(cwd, sessionID string) (int, bool, error) { return 0, true, nil }
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

// TestRun_UnconfirmedCompactionEscalation_PersistsRepair asserts the
// half of the escalation a waitForFinish unit test structurally cannot: that
// the error it returns is carried by Run's per-result handling all the way to
// the ticket file, as paused (repair) with a reason an operator can act on.
// Without this the escalation would only end the iteration, and a stuck agent
// would look like a run that quietly stopped.
func TestRun_UnconfirmedCompactionEscalation_PersistsRepair(t *testing.T) {
	t.Parallel()
	const epicName = "my-epic"
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
	})
	d := stuckCompactionDeps(nil)

	// The escalated iteration leaves the epic's only ticket paused (repair), so
	// the run pauses on it rather than returning.
	runUntilPaused(t, RunOptions{
		EpicName: epicName, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: "/fake/repo", SmartZone: 100,
	}, d, noopEventSink{})

	contents := readTicket(t, scratchDir, epicName, "01-first.md")
	assertRawPaused(t, contents, schema.PauseClassRepair, schema.PauseIterationError)
	if strings.Contains(contents, "status: done") {
		t.Errorf("ticket after escalation =\n%s\nmust not be done", contents)
	}
	if !strings.Contains(contents, errCompactRecoveryExhausted.Error()) ||
		!strings.Contains(contents, errCompactNeverConfirmed.Error()) {
		t.Errorf("ticket after escalation =\n%s\nwant a reason naming the unconfirmed compaction, not a generic recovery failure", contents)
	}
}

// TestRun_OrdinaryIterationError_KeepsItsOwnRepairReason pins the
// discrimination: the escalation reason is specific to a compaction that never
// confirmed, so an unrelated failure at the same point in the loop must not
// acquire it.
func TestRun_OrdinaryIterationError_KeepsItsOwnRepairReason(t *testing.T) {
	t.Parallel()
	const epicName = "my-epic"
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
	})
	d := stuckCompactionDeps(errors.New("herdr pane vanished"))

	runUntilPaused(t, RunOptions{
		EpicName: epicName, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: "/fake/repo", SmartZone: 100,
	}, d, noopEventSink{})

	contents := readTicket(t, scratchDir, epicName, "01-first.md")
	assertRawPaused(t, contents, schema.PauseClassRepair, schema.PauseIterationError)
	if !strings.Contains(contents, "herdr pane vanished") {
		t.Errorf("ticket after ordinary failure =\n%s\nwant needs-repair carrying the failure's own reason", contents)
	}
	if strings.Contains(contents, errCompactRecoveryExhausted.Error()) ||
		strings.Contains(contents, errCompactNeverConfirmed.Error()) {
		t.Errorf("ticket after ordinary failure =\n%s\nmust not claim an unconfirmed compaction", contents)
	}
}
