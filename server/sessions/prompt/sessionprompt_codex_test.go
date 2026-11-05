package prompt_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startCodexPromptSession runs a session on the fake Codex, Idle, with extraEnv
// added to the env it runs in. The conversation id is per test: the fake's
// control FIFO is keyed by it, so parallel tests sharing one id would steal
// each other's controls.
func startCodexPromptSession(t *testing.T, extraEnv ...string) promptFixture {
	t.Helper()
	return startCodexPromptSessionWithGrace(t, firstPromptGrace, extraEnv...)
}

// startCodexPromptSessionWithGrace is startCodexPromptSession with a longer
// wait for an answer to take effect, for a test whose fake does real work first.
func startCodexPromptSessionWithGrace(t *testing.T, grace time.Duration, extraEnv ...string) promptFixture {
	t.Helper()
	return fromPrompt(servertest.StartCodexPromptSessionWithGrace(t, grace, extraEnv...))
}

func TestSessionPrompt_CodexIsRefusedWhileWorkingAndTypedAtIdle(t *testing.T) {
	t.Parallel()
	f := startCodexPromptSession(t)
	if res := f.prompt(t, "line one\nline two"); res.Refused {
		t.Fatalf("prompt at idle = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("line one\nline two")+"\n")
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	if res := f.prompt(t, "again"); !res.Refused || res.Reason != wire.ReasonSessionNotIdle {
		t.Fatalf("prompt while working = %+v, want %s", res, wire.ReasonSessionNotIdle)
	}
}

func TestSessionPrompt_CodexPastesImagesBeforeTheText(t *testing.T) {
	t.Parallel()
	f := startCodexPromptSession(t)
	ids, paths := f.upload(t, 2)
	if res := f.promptImages(t, "see these", ids...); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	// The fake Codex does not turn on bracketed paste, so the pastes arrive
	// unmarked; the order is what this case pins. Its turn starts late, so the
	// nudge may submit the same prompt again.
	want := strconv.Quote(strings.Join(paths, "")+" see these") + "\n"
	if !servertest.Poll(10*time.Second, func() bool { return strings.HasPrefix(submitted(t, f.conv), want) }) {
		t.Fatalf("submitted = %q, want it to start with %q", submitted(t, f.conv), want)
	}
}

func TestSessionPrompt_CodexSkillTextKeepsTheNudgePath(t *testing.T) {
	t.Parallel()
	f := startCodexPromptSession(t)
	if res := f.prompt(t, "$gx-x do it"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("$gx-x do it")+"\n")
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
}

// A Codex turn that leaves a background terminal reaches the snapshot as
// Working with Background (the Agents row shows +bg from that), and it takes a
// prompt.
func TestSessionPrompt_CodexBackgroundAgentIsMarkedAndTakesAPrompt(t *testing.T) {
	t.Parallel()
	f := startCodexPromptSession(t)
	if res := f.prompt(t, "start it"); res.Refused {
		t.Fatalf("prompt at idle = %+v", res)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	for _, c := range []agentfake.Control{
		{Action: agentfake.ActionBackground, Running: true},
		{Action: agentfake.ActionFinish},
	} {
		if err := agentfake.SendProgramControl(f.conv, c); err != nil {
			t.Fatal(err)
		}
	}
	s := waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.Background })
	if s.Status.State != wire.SessionWorking {
		t.Errorf("state = %s, want working with the background mark", s.Status.State)
	}
	if res := f.prompt(t, "and then"); res.Refused {
		t.Fatalf("prompt to a background agent = %+v", res)
	}
}
