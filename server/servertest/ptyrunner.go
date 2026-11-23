package servertest

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/runnerbuild"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

// The PTY runner's timings in a harness, short so a test waits milliseconds.
const (
	ptyPollInterval = 20 * time.Millisecond
	ptyStartTimeout = 10 * time.Second
	ptyPromptGrace  = 5 * time.Second
	ptyStopGrace    = 2 * time.Second
)

// launchGraceEnv carries WithLaunchGrace to buildPty through the server env.
const launchGraceEnv = "GX_SERVERTEST_LAUNCH_GRACE"

// WithLaunchGrace shortens how long the PTY runner resends a launch prompt the
// transcript does not show, for a test whose fake never takes it.
func WithLaunchGrace(d time.Duration) func(*server.Config) {
	return WithEnv(launchGraceEnv + "=" + d.String())
}

// launchCaps is the launch proof's caps for env: a set WithLaunchGrace shortens
// the grace and the resend interval with it.
func launchCaps(env []string) deliver.LaunchTimings {
	for _, kv := range slices.Backward(env) {
		if v, ok := strings.CutPrefix(kv, launchGraceEnv+"="); ok {
			d, err := time.ParseDuration(v)
			if err != nil {
				return deliver.LaunchTimings{}
			}
			return deliver.LaunchTimings{Grace: d, Window: d, RetypeEvery: d / 4}
		}
	}
	return deliver.LaunchTimings{}
}

// ptyRunner is a nativerunner.Pty whose prompts run the harness's OnTurn hook.
// It embeds the Pty so its optional interfaces (Reattach, Nudger) still promote.
type ptyRunner struct {
	*nativerunner.Pty
	h *Harness

	mu     sync.Mutex
	agents map[string]agentInfo // label -> how the agent started
}

type agentInfo struct{ cwd, kind string }

func (r *ptyRunner) agent(label string) agentInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.agents[label]
}

func (r *ptyRunner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	if failed := r.h.failStart.Swap(nil); failed != nil {
		return agentrunner.Session{}, *failed
	}
	s, err := r.Pty.Start(opts)
	if err == nil {
		r.mu.Lock()
		r.agents[s.Label] = agentInfo{cwd: opts.Cwd, kind: string(opts.Kind)}
		r.mu.Unlock()
		r.h.recordStart(AgentStart{Label: s.Label, Cwd: opts.Cwd, Kind: string(opts.Kind)})
	}
	return s, err
}

// Stop counts the stop, then waits out a BlockStops gate before stopping.
func (r *ptyRunner) Stop(s agentrunner.Session) error {
	r.h.stops.Add(1)
	if gate := r.h.stopGate.Load(); gate != nil {
		<-*gate
	}
	return r.Pty.Stop(s)
}

func (r *ptyRunner) Prompt(s agentrunner.Session, text string) error {
	if err := r.h.takeFailedPrompt(); err != nil {
		return err
	}
	return r.PromptNudged(s, text, nil)
}

// PromptNudged runs the turn hook, then arms the TUI fake's next turn to start
// and end at once, which the typed prompt then runs. A turn control keeps the
// turn open instead, and its follow-up goes in once the prompt is.
func (r *ptyRunner) PromptNudged(s agentrunner.Session, text string, onNudge func(string)) error {
	return r.promptWith(s, text, onNudge, r.Pty.PromptNudged)
}

// PromptLaunch is PromptNudged for a launch prompt. With no OnTurn hook, a
// Claude launch prompt still arms a fast turn, so the launch sees a real turn
// instead of the pid watcher's startup count. A hook (Turn.Stall) or the fake's
// hang env keeps a test's stall.
func (r *ptyRunner) PromptLaunch(ctx context.Context, s agentrunner.Session, text string, onNudge func(string)) error {
	if r.h.turn.Load() == nil && r.agent(s.Label).kind == "claude" {
		if err := agentfake.SendTUIControl(r.h.Env("HOME"), s.SessionID, agentfake.Control{Action: agentfake.ActionFast}); err != nil {
			return err
		}
	}
	return r.promptWith(s, text, onNudge, func(s agentrunner.Session, text string, onNudge func(string)) error {
		return r.Pty.PromptLaunch(ctx, s, text, onNudge)
	})
}

// promptWith runs the harness's turn hook, if any, then sends the prompt.
func (r *ptyRunner) promptWith(s agentrunner.Session, text string, onNudge func(string), send func(agentrunner.Session, string, func(string)) error) (err error) {
	home := r.h.Env("HOME")
	if turn := r.h.turn.Load(); turn != nil {
		t := newTurn(r.h, text, r.agent(s.Label).cwd, s.SessionID)
		(*turn)(t)
		arm := agentfake.ActionFast
		if t.control.busy {
			arm = agentfake.ActionBusy
		}
		if err := agentfake.SendTUIControl(home, s.SessionID, agentfake.Control{Action: arm}); err != nil {
			return err
		}
		if after := t.control.after; after != nil {
			defer func() {
				if err == nil {
					err = agentfake.SendTUIControl(home, s.SessionID, *after)
				}
			}()
		}
	}
	return send(s, text, onNudge)
}

// buildPty builds the harness's runners the way production does, over the
// test binary as holder and as the TUI fake. serve calls it on every start, so
// a Restart gets fresh runners that reattach to the live holders.
func (h *Harness) buildPty() {
	requireSessionProcesses()
	exe := os.Args[0]
	// The fake gives the launch record by default; a test's own env in cfg.Env
	// still wins.
	env := append(h.cfg.Env[:len(h.cfg.Env):len(h.cfg.Env)], agentfake.TUIEnv+"=1")
	_, runnerFor := runnerbuild.New("pty", env, h.StateDir, h.cfg.RunnerWait, runnerbuild.Options{}, &runnerbuild.PtyOverride{
		Holder:       []string{exe, holderArg},
		Claude:       exe,
		Procs:        NewProcessTable(),
		PollInterval: ptyPollInterval,
		StartTimeout: ptyStartTimeout,
		PromptGrace:  ptyPromptGrace,
		StopGrace:    ptyStopGrace,
		LaunchCaps:   launchCaps(env),
	})
	var mu sync.Mutex
	wrapped := map[*nativerunner.Pty]*ptyRunner{}
	wrap := func(r agentrunner.Runner) agentrunner.Runner {
		p, ok := r.(*nativerunner.Pty)
		if !ok {
			return r
		}
		mu.Lock()
		defer mu.Unlock()
		if w, ok := wrapped[p]; ok {
			return w
		}
		w := &ptyRunner{Pty: p, h: h, agents: map[string]agentInfo{}}
		wrapped[p] = w
		return w
	}
	h.cfg.Runner = wrap(runnerFor("pty", runnerbuild.FallbackProject))
	h.cfg.RunnerFor = func(name, project string) agentrunner.Runner {
		return wrap(runnerFor(name, project))
	}
}

// KillAgents ends every live agent of tg's epic without counting a Stop, so
// the runner no longer finds them. PTY only.
func (h *Harness) KillAgents(t *testing.T, tg Target) {
	t.Helper()
	if !h.ptyOnly("KillAgents") {
		return
	}
	p := h.cfg.RunnerFor("pty", tg.Project).(*ptyRunner).Pty
	live, err := p.List(tg.Epic)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range live {
		if err := p.Stop(s); err != nil {
			t.Fatal(err)
		}
	}
}

// stopAgents stops every holder under the state dir's agents roots.
func (h *Harness) stopAgents() {
	roots, _ := os.ReadDir(filepath.Join(h.StateDir, "agents"))
	for _, e := range roots {
		stopSessionHolders(filepath.Join(h.StateDir, "agents", e.Name()))
	}
}
