package startprompt

import (
	"context"
	"fmt"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/internal/fakes"
)

// vimRunner answers the n-th launch prompt (1-based) with script[n-1]; past
// the script it delivers. It records every launch and whether the check was
// skipped on it.
type vimRunner struct {
	agentrunner.Runner
	script  []error
	prompts *int
	starts  *int
	skipped *[]bool
}

func (v vimRunner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	*v.starts++
	return v.Runner.Start(opts)
}

func (v vimRunner) PromptLaunch(ctx context.Context, s agentrunner.Session, text string, _ func(string)) error {
	*v.prompts++
	*v.skipped = append(*v.skipped, agentrunner.VimCheckSkipped(ctx))
	if *v.prompts <= len(v.script) && v.script[*v.prompts-1] != nil {
		return v.script[*v.prompts-1]
	}
	return v.Prompt(s, text)
}

var errVim = &claudeui.VimEditorError{Footer: "-- INSERT --"}

func launchVim(t *testing.T, script ...error) (res launchResult, starts int, skipped []bool) {
	t.Helper()
	d, _, _ := fakes.FakeDeps()
	prompts := 0
	res = launchOn(t, vimRunner{Runner: d.Runner, script: script, prompts: &prompts, starts: &starts, skipped: &skipped})
	return res, starts, skipped
}

func TestStartAndPrompt_VimErrorOnceRelaunchesOnceAndPromptsOnce(t *testing.T) {
	t.Parallel()
	res, starts, skipped := launchVim(t, errVim)

	if res.err != nil {
		t.Fatalf("StartAndPrompt = %v", res.err)
	}
	if starts != 2 || len(skipped) != 2 {
		t.Fatalf("starts = %d, launch prompts = %d; want 2 and 2", starts, len(skipped))
	}
	if skipped[0] || skipped[1] {
		t.Errorf("check skipped = %v; want the relaunch to check again", skipped)
	}
}

func TestStartAndPrompt_VimErrorOnEveryLaunchRelaunchesOnceThenTypesAnyway(t *testing.T) {
	t.Parallel()
	res, starts, skipped := launchVim(t, errVim, errVim)

	if res.err != nil {
		t.Fatalf("StartAndPrompt = %v", res.err)
	}
	if starts != 2 {
		t.Errorf("starts = %d; want 2 (one vim relaunch only)", starts)
	}
	if len(skipped) != 3 || skipped[0] || skipped[1] || !skipped[2] {
		t.Errorf("check skipped per prompt = %v; want [false false true]", skipped)
	}
}

func TestStartAndPrompt_VimRelaunchThenHungPanesTotalsThreeLaunches(t *testing.T) {
	t.Parallel()
	hung := fmt.Errorf("%w: hung", agentrunner.ErrNotDelivered)
	res, starts, _ := launchVim(t, errVim, hung, hung)

	assertLaunchFailure(t, res.err, events.AgentPromptStalled)
	if starts != 3 {
		t.Errorf("starts = %d; want 3 (vim relaunch plus two hung-pane attempts)", starts)
	}
}

func TestStartAndPrompt_PersistentVimThenUndeliveredPromptStillEndsPromptStalled(t *testing.T) {
	t.Parallel()
	notDelivered := fmt.Errorf("%w: no turn", agentrunner.ErrNotDelivered)
	res, starts, _ := launchVim(t, errVim, errVim, notDelivered, notDelivered)

	assertLaunchFailure(t, res.err, events.AgentPromptStalled)
	if starts != 3 {
		t.Errorf("starts = %d; want 3", starts)
	}
}
