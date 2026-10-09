package ralphloop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

func testIterationParams() iterationParams {
	return iterationParams{
		WorkspaceID:     "ws-1",
		RepoDir:         "/fake/repo",
		WorktreeDir:     "/fake/worktrees",
		FeatureWorktree: "/fake/repo",
		FeatureBranch:   "feature",
		Agent:           AgentClaude,
		Ticket:          tickets.Ticket{Identifier: "04"},
		WorktreeLock:    &sync.Mutex{},
		Gate:            NewGate(),
		Sink:            noopEventSink{},
	}
}

const launchEpic = "my-epic"

var launchLabel = iterLabel(launchEpic, "01")

// runLaunchEpic runs a one-ticket epic on d, until it parks if parks is set,
// and returns its scratch dir.
func runLaunchEpic(t *testing.T, d Deps, parks bool) string {
	t.Helper()
	scratchDir := writeEpic(t, launchEpic, map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
	})
	opts := RunOptions{EpicName: launchEpic, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}
	if parks {
		runUntilParked(t, opts, d, noopEventSink{})
	} else if err := Run(opts, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return scratchDir
}

func assertTicketStatus(t *testing.T, scratchDir, status string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(scratchDir, launchEpic, "issues", "01-first.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: "+status) {
		t.Errorf("ticket not %s:\n%s", status, raw)
	}
}

func launchFailedEvents(t *testing.T, scratchDir string) []Event {
	t.Helper()
	evs, ok, err := ReadEvents(scratchDir, launchEpic)
	if !ok || err != nil {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	var failed []Event
	for _, ev := range evs {
		if ev.Type == string(events.LaunchFailed) {
			failed = append(failed, ev)
		}
	}
	return failed
}

func TestRunIteration_PromptNotDelivered_RetriesOnFreshSession(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.FailNextPrompts(launchLabel, 1, agentrunner.ErrNotDelivered)

	scratchDir := runLaunchEpic(t, d, false)

	assertTicketStatus(t, scratchDir, "done")
	if got := r.Prompts(launchLabel); len(got) != 1 {
		t.Errorf("delivered prompts = %v, want one on the fresh session", got)
	}
	failed := launchFailedEvents(t, scratchDir)
	if len(failed) != 1 || failed[0].Kind != string(events.AgentPromptStalled) || failed[0].Attempt != 1 {
		t.Errorf("launch-failed events = %+v, want one agent_prompt_stalled for attempt 1", failed)
	}
}

func TestRunIteration_PromptNotDeliveredTwice_ParksPromptStalled(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.FailNextPrompts(launchLabel, 2, agentrunner.ErrNotDelivered)

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketStatus(t, scratchDir, "needs-repair")
	if _, found, _ := r.Find(launchLabel); found {
		t.Error("an undelivered session is still live, want both stopped")
	}
	failed := launchFailedEvents(t, scratchDir)
	if len(failed) != 2 {
		t.Fatalf("launch-failed events = %d, want 2", len(failed))
	}
	for i, ev := range failed {
		if ev.Kind != string(events.AgentPromptStalled) || ev.Attempt != i+1 || ev.Label != launchLabel {
			t.Errorf("event %d = kind %q attempt %d label %q", i, ev.Kind, ev.Attempt, ev.Label)
		}
	}
}

func TestRunIteration_PromptNotReady_ParksBlockedPane(t *testing.T) {
	t.Parallel()
	d, _, removed := fakeDeps()
	r := fakeRunner(d)
	// The agent is blocked on a dialog by the time the initial prompt arrives,
	// so the runner reports ErrNotReady and then stays blocked.
	onRunnerPrompt(d, func(s agentrunner.Session, _ string) error {
		r.SetState(s.Label, agentrunner.StateBlocked, "trust_directory")
		return nil
	})

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketStatus(t, scratchDir, "needs-answer")
	if _, found, _ := r.Find(launchLabel); !found {
		t.Error("blocked session was stopped, want it live for a person to answer")
	}
	if len(*removed) != 0 {
		t.Errorf("removed worktrees = %v, want the parked worktree kept", *removed)
	}
	if failed := launchFailedEvents(t, scratchDir); len(failed) != 0 {
		t.Errorf("launch-failed events = %+v, want none for a park", failed)
	}
}

func TestRunIteration_MissingCapability_ParksWithDoctorHint(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.FailNextPrompts(launchLabel, 1, fmt.Errorf("%w: msg_lifecycle_v1", agentrunner.ErrMissingCapability))

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketStatus(t, scratchDir, "needs-repair")
	failed := launchFailedEvents(t, scratchDir)
	if len(failed) != 1 {
		t.Fatalf("launch-failed events = %+v, want one", failed)
	}
	for _, want := range []string{"msg_lifecycle_v1", "gx claude doctor"} {
		if !strings.Contains(failed[0].Reason, want) {
			t.Errorf("reason %q lacks %q", failed[0].Reason, want)
		}
	}
}

func TestRunIteration_OtherPromptError_ParksWithoutRetry(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.FailNextPrompts(launchLabel, 1, errors.New("boom"))

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketStatus(t, scratchDir, "needs-repair")
	if _, found, _ := r.Find(launchLabel); !found {
		t.Error("session was stopped, want it left for needs-repair inspection")
	}
	failed := launchFailedEvents(t, scratchDir)
	if len(failed) != 1 || failed[0].Kind != string(events.IterationError) {
		t.Errorf("launch-failed events = %+v, want one iteration_error", failed)
	}
}
