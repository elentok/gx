package servertest

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/runnerbuild"
	"github.com/elentok/gx/testutil/agentfake"
)

// The PTY runner's timings in a harness, short so a test waits milliseconds.
const (
	ptyPollInterval = 20 * time.Millisecond
	ptyStartTimeout = 10 * time.Second
	ptyPromptGrace  = 500 * time.Millisecond
	ptyStopGrace    = 2 * time.Second
)

// ptyRunner is a nativerunner.Pty whose prompts run the harness's OnTurn hook.
// It embeds the Pty so its optional interfaces (Reattach, Nudger) still promote.
type ptyRunner struct {
	*nativerunner.Pty
	h *Harness

	mu   sync.Mutex
	cwds map[string]string // label -> the cwd the agent started in
}

func (r *ptyRunner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	if failed := r.h.failStart.Swap(nil); failed != nil {
		return agentrunner.Session{}, *failed
	}
	s, err := r.Pty.Start(opts)
	if err == nil {
		r.mu.Lock()
		r.cwds[s.Label] = opts.Cwd
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
	return r.PromptNudged(s, text, nil)
}

// PromptNudged runs the turn hook, then arms the TUI fake's next turn to start
// and end at once, which the typed prompt then runs. A turn control keeps the
// turn open instead, and its follow-up goes in once the prompt is.
func (r *ptyRunner) PromptNudged(s agentrunner.Session, text string, onNudge func(string)) (err error) {
	home := r.h.Env("HOME")
	if turn := r.h.turn.Load(); turn != nil {
		r.mu.Lock()
		cwd := r.cwds[s.Label]
		r.mu.Unlock()
		_, rest, _ := strings.Cut(text, " ")
		addr, _, _ := strings.Cut(rest, "\n")
		addr = strings.TrimPrefix(addr, "--ticket ") // gx-merge's form
		t := &Turn{Address: addr, Cwd: cwd, Text: text, h: r.h, sessionID: s.SessionID}
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
	return r.Pty.PromptNudged(s, text, onNudge)
}

// buildPty builds the harness's runners the way production does, over the
// test binary as holder and as the TUI fake. serve calls it on every start, so
// a Restart gets fresh runners that reattach to the live holders.
func (h *Harness) buildPty() {
	exe := os.Args[0]
	env := append(h.cfg.Env[:len(h.cfg.Env):len(h.cfg.Env)], agentfake.TUIEnv+"=1")
	_, runnerFor := runnerbuild.New("pty", env, h.StateDir, h.cfg.RunnerWait, &runnerbuild.PtyOverride{
		Holder:       []string{exe, holderArg},
		Claude:       exe,
		Procs:        NewProcessTable(),
		PollInterval: ptyPollInterval,
		StartTimeout: ptyStartTimeout,
		PromptGrace:  ptyPromptGrace,
		StopGrace:    ptyStopGrace,
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
		w := &ptyRunner{Pty: p, h: h, cwds: map[string]string{}}
		wrapped[p] = w
		return w
	}
	h.cfg.Runner = wrap(runnerFor("pty", runnerbuild.FallbackProject))
	h.cfg.RunnerFor = func(name, project string) agentrunner.Runner { return wrap(runnerFor(name, project)) }
}

// stopAgents stops every holder under the state dir's agents roots.
func (h *Harness) stopAgents() {
	roots, _ := os.ReadDir(filepath.Join(h.StateDir, "agents"))
	for _, e := range roots {
		stopSessionHolders(filepath.Join(h.StateDir, "agents", e.Name()))
	}
}
