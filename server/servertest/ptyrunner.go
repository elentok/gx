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
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

// The PTY runner's timings in a harness, short so a test waits milliseconds.
const (
	ptyPollInterval = 20 * time.Millisecond
	ptyStartTimeout = 10 * time.Second
	ptyPromptGrace  = 500 * time.Millisecond
	ptyStopGrace    = 2 * time.Second
)

// withPty is the expand step of the PTY default: it makes the PTY runner the
// configured one, for servertest's own tests. The PTY default ticket makes it
// the default branch and removes this.
func withPty() func(*server.Config) {
	return func(c *server.Config) { choiceOf(c).pty = true }
}

// ptyRunner is a nativerunner.Pty whose prompts run the harness's OnTurn hook.
// It embeds the Pty so its optional interfaces (Reattach, Nudger) still promote.
type ptyRunner struct {
	*nativerunner.Pty
	h *Harness

	mu   sync.Mutex
	cwds map[string]string // label -> the cwd the agent started in
}

func (r *ptyRunner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	s, err := r.Pty.Start(opts)
	if err == nil {
		r.mu.Lock()
		r.cwds[s.Label] = opts.Cwd
		r.mu.Unlock()
	}
	return s, err
}

func (r *ptyRunner) Prompt(s agentrunner.Session, text string) error {
	return r.PromptNudged(s, text, nil)
}

// PromptNudged runs the turn hook, then arms the TUI fake's next turn to start
// and end at once, which the typed prompt then runs.
func (r *ptyRunner) PromptNudged(s agentrunner.Session, text string, onNudge func(string)) error {
	if turn := r.h.turn.Load(); turn != nil {
		r.mu.Lock()
		cwd := r.cwds[s.Label]
		r.mu.Unlock()
		_, rest, _ := strings.Cut(text, " ")
		addr, _, _ := strings.Cut(rest, "\n")
		addr = strings.TrimPrefix(addr, "--ticket ") // gx-merge's form
		(*turn)(&Turn{Address: addr, Cwd: cwd, Text: text})
		if err := agentfake.SendTUIControl(r.h.Env("HOME"), s.SessionID, agentfake.Control{Action: agentfake.ActionFast}); err != nil {
			return err
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
