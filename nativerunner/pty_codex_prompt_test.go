package nativerunner_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	gxharness "github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

// codexPromptSession starts an idle fake Codex with a known conversation id and
// returns it with its holder.
func codexPromptSession(t *testing.T, id string) (*nativerunner.Pty, agentrunner.Session, *nativerunner.HolderClient) {
	t.Helper()
	p, _ := codexRunner(t, agentfake.CodexIDEnv+"="+id)
	s := startCodex(t, p, realDir(t))
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	holder, err := nativerunner.DialHolder(filepath.Join(p.Root, "e-01"))
	if err != nil {
		t.Fatal(err)
	}
	return p, s, holder
}

// waitWatched polls the holder's watched status until ok accepts it.
func waitWatched(t *testing.T, holder *nativerunner.HolderClient, ok func(gxharness.Status) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w, err := holder.Watched()
		if err != nil {
			t.Fatal(err)
		}
		if w != nil && ok(w.Status) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("watched status = %+v, never matched", w)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func isWorking(st gxharness.Status) bool    { return st.State == gxharness.Working && !st.Background }
func isBackground(st gxharness.Status) bool { return st.State == gxharness.Working && st.Background }

// submittedEntries reads the fake's log of submitted inputs.
func submittedEntries(t *testing.T, id string) []string {
	t.Helper()
	data, err := os.ReadFile(agentfake.ProgramSubmittedPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var entry string
		if _, err := fmt.Sscanf(line, "%q", &entry); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		out = append(out, entry)
	}
	return out
}

func waitSubmitted(t *testing.T, id string, want ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := submittedEntries(t, id)
		if slices.Equal(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("submitted = %q, want %q", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPtyCodex_PromptAtIdleIsDelivered(t *testing.T) {
	t.Parallel()
	p, s, holder := codexPromptSession(t, "thread-prompt")
	if err := p.Prompt(s, "fix the bug"); err != nil {
		t.Fatal(err)
	}
	waitSubmitted(t, "thread-prompt", "fix the bug")
	waitWatched(t, holder, isWorking)
}

func TestPtyCodex_MultiLinePromptIsOneSubmittedEntry(t *testing.T) {
	t.Parallel()
	p, s, _ := codexPromptSession(t, "thread-multi")
	text := "line one\n\nline three"
	if err := p.Prompt(s, text); err != nil {
		t.Fatal(err)
	}
	waitSubmitted(t, "thread-multi", text)
}

func TestPtyCodex_PromptWhileWorkingIsNotReady(t *testing.T) {
	t.Parallel()
	p, s, holder := codexPromptSession(t, "thread-busy")
	if err := p.Prompt(s, "first"); err != nil {
		t.Fatal(err)
	}
	waitWatched(t, holder, isWorking)
	if err := p.Prompt(s, "second"); !errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Prompt while working = %v, want ErrNotReady", err)
	}
	waitSubmitted(t, "thread-busy", "first")
}

func TestPtyCodex_BackgroundAgentTakesAPromptAndIsNotIdle(t *testing.T) {
	t.Parallel()
	p, s, holder := codexPromptSession(t, "thread-bgprompt")
	if err := p.Prompt(s, "start it"); err != nil {
		t.Fatal(err)
	}
	waitWatched(t, holder, isWorking)
	for _, c := range []agentfake.Control{
		{Action: agentfake.ActionBackground, Running: true},
		{Action: agentfake.ActionFinish},
	} {
		if err := agentfake.SendProgramControl("thread-bgprompt", c); err != nil {
			t.Fatal(err)
		}
	}
	waitWatched(t, holder, isBackground)
	if st, err := p.Status(s); err != nil || st.State == agentrunner.StateIdle {
		t.Fatalf("Status = %+v, %v; a background agent is not idle", st, err)
	}
	if err := p.Prompt(s, "and then"); err != nil {
		t.Fatalf("Prompt to a background agent: %v", err)
	}
	waitSubmitted(t, "thread-bgprompt", "start it", "and then")
}

func TestPtyCodex_TabQueuesTextWhileWorkingAndSubmitsAfterTheTurn(t *testing.T) {
	t.Parallel()
	p, s, holder := codexPromptSession(t, "thread-queue")
	if err := p.Prompt(s, "first"); err != nil {
		t.Fatal(err)
	}
	waitWatched(t, holder, isWorking)
	if err := holder.TypeText("queued"); err != nil {
		t.Fatal(err)
	}
	if err := holder.SendKeys("\t"); err != nil {
		t.Fatal(err)
	}
	// Queued, not submitted: nothing beyond the first prompt yet.
	time.Sleep(100 * time.Millisecond)
	if got := submittedEntries(t, "thread-queue"); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("submitted while working = %q, want only the first prompt", got)
	}
	if err := agentfake.SendProgramControl("thread-queue", agentfake.Control{Action: agentfake.ActionFinish}); err != nil {
		t.Fatal(err)
	}
	waitSubmitted(t, "thread-queue", "first", "queued")
}

func TestPtyCodex_InterruptEndsTheTurnAndKeepsTheBackgroundMarker(t *testing.T) {
	t.Parallel()
	p, s, holder := codexPromptSession(t, "thread-int")
	if err := p.Prompt(s, "work"); err != nil {
		t.Fatal(err)
	}
	waitWatched(t, holder, isWorking)
	if err := agentfake.SendProgramControl("thread-int", agentfake.Control{Action: agentfake.ActionBackground, Running: true}); err != nil {
		t.Fatal(err)
	}
	if err := p.Interrupt(s); err != nil {
		t.Fatal(err)
	}
	waitWatched(t, holder, isBackground)
	// One Ctrl+C: a second would quit Codex.
	waitSubmitted(t, "thread-int", "work", agentfake.ProgramInterruptEntry)
}
