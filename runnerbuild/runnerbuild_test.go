package runnerbuild

import (
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/nativerunner"
)

func testEnv(t *testing.T) []string {
	t.Helper()
	return []string{"HOME=" + t.TempDir()}
}

func TestNew_HeadlessAgentDirsLiveUnderTheProjectsAgentsRoot(t *testing.T) {
	state := t.TempDir()
	_, runnerFor := New(agentrunner.ChoiceHeadless, testEnv(t), state, 0, nil)
	h, ok := runnerFor("headless", "proj").(*nativerunner.Headless)
	if !ok {
		t.Fatalf("runnerFor(headless, proj) = %T, want *Headless", runnerFor("headless", "proj"))
	}
	if want := nativerunner.AgentsRoot(state, "proj"); h.Root != want {
		t.Errorf("Root = %q, want %q", h.Root, want)
	}
	if runnerFor("headless", "proj") != agentrunner.Runner(h) {
		t.Error("a project must keep one Headless: its live sessions are in memory")
	}
}

func TestNew_EveryRunnerCarriesN(t *testing.T) {
	for _, tc := range []struct {
		n    time.Duration
		want time.Duration
	}{{9 * time.Second, 9 * time.Second}, {0, agentrunner.DefaultN}} {
		_, runnerFor := New(agentrunner.ChoiceHeadless, testEnv(t), t.TempDir(), tc.n, nil)
		got := map[string]time.Duration{
			"herdr":    runnerFor("herdr", "p").(*herdrrunner.Runner).WaitN(),
			"headless": runnerFor("headless", "p").(*nativerunner.Headless).WaitN(),
			"pty":      runnerFor("pty", "p").(*nativerunner.Pty).WaitN(),
		}
		for name, n := range got {
			if n != tc.want {
				t.Errorf("n=%s: %s WaitN = %s, want %s", tc.n, name, n, tc.want)
			}
		}
	}
}

func TestNew_PtyAgentDirsLiveUnderTheProjectsAgentsRoot(t *testing.T) {
	state := t.TempDir()
	r, runnerFor := New(agentrunner.ChoicePTY, testEnv(t), state, 0, nil)
	if r.Name() != "pty" {
		t.Errorf("default runner Name = %q, want pty", r.Name())
	}
	p, ok := runnerFor("pty", "proj").(*nativerunner.Pty)
	if !ok {
		t.Fatalf("runnerFor(pty, proj) = %T, want *Pty", runnerFor("pty", "proj"))
	}
	if want := nativerunner.AgentsRoot(state, "proj"); p.Root != want {
		t.Errorf("Root = %q, want %q", p.Root, want)
	}
	if runnerFor("pty", "proj") != agentrunner.Runner(p) {
		t.Error("a project must keep one Pty: its live sessions are in memory")
	}
}

func TestNew_BuildsAnyRunnerPerPairLazily(t *testing.T) {
	_, runnerFor := New(agentrunner.ChoiceHeadless, testEnv(t), t.TempDir(), 0, nil)
	herdrA := runnerFor("herdr", "a")
	if herdrA == nil || herdrA.Name() != "herdr" {
		t.Fatalf("runnerFor(herdr, a) = %v, want a herdr runner", herdrA)
	}
	if runnerFor("herdr", "a") != herdrA {
		t.Error("a (runner, project) pair must be built once and kept")
	}
	if runnerFor("herdr", "b") == herdrA {
		t.Error("each project gets its own runner")
	}
	if runnerFor("headless", "a") == herdrA {
		t.Error("each runner name gets its own runner")
	}
	if r := runnerFor("tmux", "a"); r != nil {
		t.Errorf("unknown runner = %v, want nil", r)
	}
}

func TestNew_HeadlessDefaultRunnerHasARoot(t *testing.T) {
	r, _ := New(agentrunner.ChoiceHeadless, testEnv(t), t.TempDir(), 0, nil)
	if h, ok := r.(*nativerunner.Headless); !ok || h.Root == "" {
		t.Errorf("default runner = %#v, want a *Headless with a Root", r)
	}
}

func TestNew_NamesComeFromTheRunners(t *testing.T) {
	if r, _ := New(agentrunner.ChoiceHerdr, testEnv(t), t.TempDir(), 0, nil); r.Name() != "herdr" {
		t.Errorf("herdr Name = %q", r.Name())
	}
	if r, _ := New(agentrunner.ChoiceHeadless, testEnv(t), t.TempDir(), 0, nil); r.Name() != "headless" {
		t.Errorf("headless Name = %q", r.Name())
	}
}

func TestNew_HerdrServerCanBuildHeadless(t *testing.T) {
	_, runnerFor := New(agentrunner.ChoiceHerdr, testEnv(t), t.TempDir(), 0, nil)
	if _, ok := runnerFor("headless", "proj").(*nativerunner.Headless); !ok {
		t.Error("a herdr server must still reach a headless run's runner")
	}
}

type fakeProcs struct{ nativerunner.ProcessTable }

func TestNew_PtyOverrideReachesThePty(t *testing.T) {
	override := &PtyOverride{
		Holder:       []string{"gx", "agent-holder"},
		Claude:       "/bin/fake-claude",
		Codex:        "/bin/fake-codex",
		Procs:        fakeProcs{},
		PollInterval: time.Millisecond,
		StartTimeout: 2 * time.Millisecond,
		PromptGrace:  3 * time.Millisecond,
		StopGrace:    4 * time.Millisecond,
	}
	_, runnerFor := New(agentrunner.ChoicePTY, testEnv(t), t.TempDir(), 0, override)
	p := runnerFor("pty", "proj").(*nativerunner.Pty)
	if !reflect.DeepEqual(p.Holder, override.Holder) || p.Claude != override.Claude || p.Codex != override.Codex || p.Procs != override.Procs {
		t.Errorf("holder/claude/codex/procs = %v %q %q %v, want the override's", p.Holder, p.Claude, p.Codex, p.Procs)
	}
	if p.PollInterval != time.Millisecond || p.StartTimeout != 2*time.Millisecond ||
		p.PromptGrace != 3*time.Millisecond || p.StopGrace != 4*time.Millisecond {
		t.Errorf("timings = %v %v %v %v, want 1ms 2ms 3ms 4ms", p.PollInterval, p.StartTimeout, p.PromptGrace, p.StopGrace)
	}
}

func TestNew_NoPtyOverrideLeavesThePtyFieldsEmpty(t *testing.T) {
	_, runnerFor := New(agentrunner.ChoicePTY, testEnv(t), t.TempDir(), 0, nil)
	p := runnerFor("pty", "proj").(*nativerunner.Pty)
	if p.Holder != nil || p.Claude != "" || p.Codex != "" || p.Procs != nil ||
		p.PollInterval != 0 || p.StartTimeout != 0 || p.PromptGrace != 0 || p.StopGrace != 0 {
		t.Errorf("pty = %+v, want holder, agent, procs and timings unset", p)
	}
}
