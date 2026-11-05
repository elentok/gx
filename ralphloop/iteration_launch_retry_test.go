package ralphloop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fakes"
)

const launchEpic = "my-epic"

var launchLabel = iterLabel(launchEpic, "01")

// launchResult is what the server's launch sees from StartAndPrompt: the
// session, the error, and every nudge decision as (attempt, decision).
type launchResult struct {
	session agentrunner.Session
	err     error
	nudges  []string
}

// launchOn starts the launch epic's ticket on r the way the server's launch
// does.
func launchOn(t *testing.T, r agentrunner.Runner) launchResult {
	t.Helper()
	var res launchResult
	res.session, res.err = StartAndPrompt(context.Background(), r, agentrunner.StartOptions{Label: launchLabel, Epic: launchEpic}, "/gx-implement gx:my-epic/01",
		func(attempt int, decision string) {
			res.nudges = append(res.nudges, fmt.Sprintf("%d:%s", attempt, decision))
		})
	return res
}

// assertLaunchFailure checks err is a launch failure of kind.
func assertLaunchFailure(t *testing.T, err error, kind events.Kind) {
	t.Helper()
	if got, ok := LaunchFailureKind(err); !ok || got != kind {
		t.Fatalf("launch failure kind = %q (found %v) from %v, want %q", got, ok, err, kind)
	}
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

func TestStartAndPrompt_ReportsEveryNudgeDecision(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()

	res := launchOn(t, nudgingRunner{Runner: d.Runner, decisions: []string{"retype", "enter"}})

	if res.err != nil {
		t.Fatalf("StartAndPrompt = %v", res.err)
	}
	if want := []string{"1:retype", "1:enter"}; !slices.Equal(res.nudges, want) {
		t.Fatalf("nudges = %v, want %v", res.nudges, want)
	}
}

// launchRunner counts launch prompts and rejects the first rejects of them.
// Its plain Prompt always delivers, so only the launch method can fail.
type launchRunner struct {
	agentrunner.Runner
	rejects  int
	launches *int
}

func (l launchRunner) PromptLaunch(_ context.Context, s agentrunner.Session, text string, _ func(string)) error {
	*l.launches++
	if *l.launches <= l.rejects {
		return agentrunner.ErrNotDelivered
	}
	return l.Prompt(s, text)
}

// unknownSkillRunner rejects every launch prompt as an unknown skill.
type unknownSkillRunner struct{ agentrunner.Runner }

func (unknownSkillRunner) PromptLaunch(_ context.Context, _ agentrunner.Session, text string, _ func(string)) error {
	return fmt.Errorf("%w: %w: %q", agentrunner.ErrNotDelivered, agentrunner.ErrUnknownSkill, text)
}

func TestStartAndPrompt_AlwaysUnknownSkill_NamesSkillAndFix(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	var kinds []events.Kind
	_, err := startAndPrompt(context.Background(), unknownSkillRunner{d.Runner}, agentrunner.StartOptions{Label: "it-01", Epic: "epic", Cwd: "/wt"}, "/gx-implement gx:epic/01",
		func(_ int, kind events.Kind, _ error) { kinds = append(kinds, kind) }, func(int, string) {})
	var lf *launchFailure
	if !errors.As(err, &lf) || lf.Kind != events.AgentPromptStalled {
		t.Fatalf("err = %v, want agent_prompt_stalled launchFailure", err)
	}
	for _, want := range []string{"/gx-implement", "gx skills install", "Retry"} {
		if !strings.Contains(lf.Error(), want) {
			t.Errorf("reason %q lacks %q", lf.Error(), want)
		}
	}
	if len(kinds) != 2 {
		t.Errorf("launch-failed kinds = %v, want two (resend, fresh-session retry)", kinds)
	}
}

func TestStartAndPrompt_LaunchPromptNotDelivered_RetriesOnFreshSession(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	launches := 0

	res := launchOn(t, launchRunner{Runner: d.Runner, rejects: 1, launches: &launches})

	if res.err != nil {
		t.Fatalf("StartAndPrompt = %v, want the fresh session to launch", res.err)
	}
	if launches != 2 {
		t.Errorf("launch prompts = %d, want 2 (rejected, then the fresh session)", launches)
	}
}

func TestStartAndPrompt_LaunchPromptNotDeliveredTwice_FailsPromptStalled(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	launches := 0

	res := launchOn(t, launchRunner{Runner: d.Runner, rejects: 2, launches: &launches})

	assertLaunchFailure(t, res.err, events.AgentPromptStalled)
	if launches != 2 {
		t.Errorf("launch prompts = %d, want 2", launches)
	}
}

func TestStartAndPrompt_PromptNotDelivered_RetriesOnFreshSession(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	r := fakes.FakeRunner(d)
	r.FailNextPrompts(launchLabel, 1, agentrunner.ErrNotDelivered)

	res := launchOn(t, r)

	if res.err != nil {
		t.Fatalf("StartAndPrompt = %v, want the fresh session to launch", res.err)
	}
	if got := r.Prompts(launchLabel); len(got) != 1 {
		t.Errorf("delivered prompts = %v, want one on the fresh session", got)
	}
}

func TestStartAndPrompt_PromptNotDeliveredTwice_FailsPromptStalledAndStopsBoth(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	r := fakes.FakeRunner(d)
	r.FailNextPrompts(launchLabel, 2, agentrunner.ErrNotDelivered)

	res := launchOn(t, r)

	assertLaunchFailure(t, res.err, events.AgentPromptStalled)
	if _, found, _ := r.Find(launchLabel); found {
		t.Error("an undelivered session is still live, want both stopped")
	}
}

func TestStartAndPrompt_MissingCapability_CarriesDoctorHint(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	r := fakes.FakeRunner(d)
	r.FailNextPrompts(launchLabel, 1, fmt.Errorf("%w: msg_lifecycle_v1", agentrunner.ErrMissingCapability))

	res := launchOn(t, r)

	if res.err == nil {
		t.Fatal("StartAndPrompt = nil, want the missing-capability error")
	}
	for _, want := range []string{"msg_lifecycle_v1", "gx claude doctor"} {
		if !strings.Contains(res.err.Error(), want) {
			t.Errorf("error %q lacks %q", res.err, want)
		}
	}
}

func TestStartAndPrompt_MissingCapabilityOnStart_FailsIterationErrorWithDoctorHint(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	r := fakes.FakeRunner(d)
	r.SetStartErr(launchLabel, fmt.Errorf("%w: pid_status_v1", agentrunner.ErrMissingCapability))

	res := launchOn(t, r)

	assertLaunchFailure(t, res.err, events.IterationError)
	for _, want := range []string{"pid_status_v1", "gx claude doctor"} {
		if !strings.Contains(res.err.Error(), want) {
			t.Errorf("error %q lacks %q", res.err, want)
		}
	}
}

func TestStartAndPrompt_MissingCapabilityOnStart_CarriesDoctorHint(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	r := fakes.FakeRunner(d)
	r.SetStartErr(launchLabel, fmt.Errorf("%w: pid_status_v1", agentrunner.ErrMissingCapability))

	_, err := StartAndPrompt(context.Background(), r, agentrunner.StartOptions{Label: launchLabel}, "/go", func(int, string) {})

	if !errors.Is(err, agentrunner.ErrMissingCapability) || !strings.Contains(err.Error(), "gx claude doctor") {
		t.Errorf("StartAndPrompt = %v, want ErrMissingCapability with the doctor hint", err)
	}
}

func TestWaitIterationFinished_SessionDied_ReturnsDiedReason(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	r := fakes.FakeRunner(d)
	r.DieAfterPrompt(launchLabel, agentrunner.SessionDied("claude exited (code 1)"))
	res := launchOn(t, r)
	if res.err != nil {
		t.Fatalf("StartAndPrompt = %v", res.err)
	}

	err := WaitIterationFinished(d, OneIteration{Agent: agent.AgentClaude, Epic: launchEpic}, IterationWorktree{Label: launchLabel}, res.session)

	if !errors.Is(err, agentrunner.ErrSessionDied) || !strings.Contains(err.Error(), "claude exited (code 1)") {
		t.Errorf("WaitIterationFinished = %v, want ErrSessionDied carrying the died reason", err)
	}
}

func TestStartAndPrompt_HolderStartFailure_FailsIterationErrorWithStderr(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	r := fakes.FakeRunner(d)
	r.SetStartErr(launchLabel, errors.New("pty start: holder exited: Error: listen unix: invalid argument"))

	res := launchOn(t, r)

	assertLaunchFailure(t, res.err, events.IterationError)
	if !strings.Contains(res.err.Error(), "listen unix: invalid argument") {
		t.Errorf("error %q lacks the holder's stderr", res.err)
	}
}

func TestStartAndPrompt_OtherPromptError_ReturnsWithoutRetryAndKeepsSession(t *testing.T) {
	t.Parallel()
	d, _, _ := fakes.FakeDeps()
	r := fakes.FakeRunner(d)
	r.FailNextPrompts(launchLabel, 1, errors.New("boom"))

	res := launchOn(t, r)

	if res.err == nil || !strings.Contains(res.err.Error(), "boom") {
		t.Fatalf("StartAndPrompt = %v, want the prompt error", res.err)
	}
	if _, ok := LaunchFailureKind(res.err); ok {
		t.Errorf("err %v is a launch failure, want a plain error with no retry", res.err)
	}
	if _, found, _ := r.Find(launchLabel); !found {
		t.Error("session was stopped, want it left for repair inspection")
	}
}
