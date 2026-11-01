package ralphloop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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

// runLaunchEpic runs a one-ticket epic on d, until it pauses if pauses is set,
// and returns its scratch dir.
func runLaunchEpic(t *testing.T, d Deps, pauses bool) string {
	t.Helper()
	scratchDir := writeEpic(t, launchEpic, map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
	})
	opts := RunOptions{EpicName: launchEpic, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}
	if pauses {
		runUntilPaused(t, opts, d, noopEventSink{})
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
	if tk := parseRawTicket(t, string(raw)); string(tk.Status) != status {
		t.Errorf("ticket status = %q, want %q", tk.Status, status)
	}
}

// assertTicketPaused checks the launch epic's ticket is paused in class.
func assertTicketPaused(t *testing.T, scratchDir string, class schema.PauseClass) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(scratchDir, launchEpic, "issues", "01-first.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), class, "")
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

// nudgingRunner reports decisions on every PromptNudged, as herdr and pty do
// on a launch prompt the agent didn't pick up at once.
type nudgingRunner struct {
	agentrunner.Runner
	decisions []string
}

func (n nudgingRunner) PromptNudged(s agentrunner.Session, text string, onNudge func(string)) error {
	for _, d := range n.decisions {
		onNudge(d)
	}
	return n.Prompt(s, text)
}

func TestRunIteration_LogsEveryNudgeDecision(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	d.Runner = nudgingRunner{Runner: d.Runner, decisions: []string{"retype", "enter"}}

	scratchDir := runLaunchEpic(t, d, false)

	evs, _, err := ReadEvents(scratchDir, launchEpic)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, ev := range evs {
		if ev.Type != string(events.PromptNudged) {
			continue
		}
		if ev.Ticket != "01" || ev.Label != launchLabel || ev.Attempt != 1 {
			t.Errorf("prompt-nudged event = %+v, want ticket 01, label %s, attempt 1", ev, launchLabel)
		}
		got = append(got, ev.Reason)
	}
	if want := []string{"retype", "enter"}; !slices.Equal(got, want) {
		t.Fatalf("prompt-nudged reasons = %v, want %v", got, want)
	}
}

// launchRunner counts launch prompts and rejects the first rejects of them.
// Its plain Prompt always delivers, so only the launch method can fail.
type launchRunner struct {
	agentrunner.Runner
	rejects  int
	launches *int
}

func (l launchRunner) PromptLaunch(s agentrunner.Session, text string, _ func(string)) error {
	*l.launches++
	if *l.launches <= l.rejects {
		return agentrunner.ErrNotDelivered
	}
	return l.Prompt(s, text)
}

func TestRunIteration_LaunchPromptNotDelivered_RetriesOnFreshSession(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	launches := 0
	d.Runner = launchRunner{Runner: d.Runner, rejects: 1, launches: &launches}

	scratchDir := runLaunchEpic(t, d, false)

	assertTicketStatus(t, scratchDir, "done")
	if launches != 2 {
		t.Errorf("launch prompts = %d, want 2 (rejected, then the fresh session)", launches)
	}
	failed := launchFailedEvents(t, scratchDir)
	if len(failed) != 1 || failed[0].Kind != string(events.AgentPromptStalled) {
		t.Errorf("launch-failed events = %+v, want one agent_prompt_stalled", failed)
	}
}

func TestRunIteration_LaunchPromptNotDeliveredTwice_PausesPromptStalled(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	launches := 0
	d.Runner = launchRunner{Runner: d.Runner, rejects: 2, launches: &launches}

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketPaused(t, scratchDir, schema.PauseClassRepair)
	if launches != 2 {
		t.Errorf("launch prompts = %d, want 2", launches)
	}
	if failed := launchFailedEvents(t, scratchDir); len(failed) != 2 {
		t.Errorf("launch-failed events = %d, want 2", len(failed))
	}
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

func TestRunIteration_PromptNotDeliveredTwice_PausesPromptStalled(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.FailNextPrompts(launchLabel, 2, agentrunner.ErrNotDelivered)

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketPaused(t, scratchDir, schema.PauseClassRepair)
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

func TestRunIteration_MissingCapability_PausesWithDoctorHint(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.FailNextPrompts(launchLabel, 1, fmt.Errorf("%w: msg_lifecycle_v1", agentrunner.ErrMissingCapability))

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketPaused(t, scratchDir, schema.PauseClassRepair)
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

func TestRunIteration_MissingCapabilityOnStart_PausesWithDoctorHint(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.SetStartErr(launchLabel, fmt.Errorf("%w: pid_status_v1", agentrunner.ErrMissingCapability))

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketPaused(t, scratchDir, schema.PauseClassRepair)
	failed := launchFailedEvents(t, scratchDir)
	if len(failed) != 1 {
		t.Fatalf("launch-failed events = %+v, want one", failed)
	}
	for _, want := range []string{"pid_status_v1", "gx claude doctor"} {
		if !strings.Contains(failed[0].Reason, want) {
			t.Errorf("reason %q lacks %q", failed[0].Reason, want)
		}
	}
}

func TestStartAndPrompt_MissingCapabilityOnStart_CarriesDoctorHint(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.SetStartErr(launchLabel, fmt.Errorf("%w: pid_status_v1", agentrunner.ErrMissingCapability))

	_, err := StartAndPrompt(r, agentrunner.StartOptions{Label: launchLabel}, "/go", func(int, string) {})

	if !errors.Is(err, agentrunner.ErrMissingCapability) || !strings.Contains(err.Error(), "gx claude doctor") {
		t.Errorf("StartAndPrompt = %v, want ErrMissingCapability with the doctor hint", err)
	}
}

func TestRunIteration_SessionDiedInWait_PausesRepairWithReason(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.DieAfterPrompt(launchLabel, agentrunner.SessionDied("claude exited (code 1)"))

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketPaused(t, scratchDir, schema.PauseClassRepair)
	evs, _, err := ReadEvents(scratchDir, launchEpic)
	if err != nil {
		t.Fatalf("ReadEvents() error = %v", err)
	}
	var paused []Event
	for _, ev := range evs {
		if ev.Type == string(events.Paused) {
			paused = append(paused, ev)
		}
	}
	if len(paused) != 1 || paused[0].Kind != string(events.IterationError) ||
		!strings.Contains(paused[0].Reason, "claude exited (code 1)") {
		t.Errorf("paused events = %+v, want one iteration_error carrying the died reason", paused)
	}
}

func TestRunIteration_OtherPromptError_PausesWithoutRetry(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	r := fakeRunner(d)
	r.FailNextPrompts(launchLabel, 1, errors.New("boom"))

	scratchDir := runLaunchEpic(t, d, true)

	assertTicketPaused(t, scratchDir, schema.PauseClassRepair)
	if _, found, _ := r.Find(launchLabel); !found {
		t.Error("session was stopped, want it left for repair inspection")
	}
	failed := launchFailedEvents(t, scratchDir)
	if len(failed) != 1 || failed[0].Kind != string(events.IterationError) {
		t.Errorf("launch-failed events = %+v, want one iteration_error", failed)
	}
}
