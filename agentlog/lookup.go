package agentlog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

var (
	// ErrNoAgent: the ticket has no native agent directory and no live herdr
	// iteration.
	ErrNoAgent = errors.New("no agent log")
	// ErrUnsupported: herdr Codex agents have no log gx can read, and watch
	// never scrapes panes.
	ErrUnsupported = errors.New("watch is not supported for herdr codex agents")
)

// Log is where one agent's log lives.
type Log struct {
	// Path is the JSONL file to read from Offset: out.jsonl for a native
	// agent, the claude transcript for a herdr one. Empty when Pruned.
	Path   string
	Offset int64
	// Ended reports whether the agent is gone, so follow knows to stop.
	Ended func() bool
	// Pruned: retention removed the native log. Transcript still names the
	// claude transcript, which claude keeps on its own schedule.
	Pruned     bool
	Transcript string
}

// HerdrIteration is what Locator needs from a live herdr iteration.
type HerdrIteration struct {
	Agent      string
	Transcript string
}

// Locator finds an agent's log: the native agent directory first, then a
// live herdr iteration.
type Locator struct {
	StateDir string
	// Procs overrides the process table native liveness checks use.
	Procs nativerunner.ProcessTable
	// Herdr looks up a live herdr iteration by full ticket address. Nil
	// means herdr isn't in play.
	Herdr func(ctx context.Context, address string) (HerdrIteration, bool, error)
}

// AgentLog finds the log of a's agent.
func (l Locator) AgentLog(ctx context.Context, a tickets.Address) (Log, error) {
	label, _, _ := ralphloop.IterationIdentity(a.Epic, a.ID, "")
	h := &nativerunner.Headless{Root: nativerunner.AgentsRoot(l.StateDir, a.Project), Procs: l.Procs}
	meta, _, err := h.Inspect(label)
	switch {
	case err == nil:
		dir := filepath.Join(h.Root, label)
		if nativerunner.Pruned(dir) {
			p, _ := transcript.Path(meta.Cwd, meta.SessionID)
			return Log{Pruned: true, Transcript: p}, nil
		}
		return Log{
			Path: filepath.Join(dir, nativerunner.OutFile), Offset: meta.Offset,
			Ended: func() bool {
				_, live, err := h.Inspect(label)
				return err == nil && !live
			},
		}, nil
	case !errors.Is(err, agentrunner.ErrNotFound):
		return Log{}, err
	}

	if l.Herdr == nil {
		return Log{}, fmt.Errorf("%w for %s", ErrNoAgent, a)
	}
	it, ok, err := l.Herdr(ctx, a.String())
	if err != nil {
		return Log{}, err
	}
	if !ok {
		return Log{}, fmt.Errorf("%w for %s", ErrNoAgent, a)
	}
	if it.Agent == string(ralphloop.AgentCodex) {
		return Log{}, ErrUnsupported
	}
	if it.Transcript == "" {
		return Log{}, fmt.Errorf("%w for %s: herdr agent has no claude session yet", ErrNoAgent, a)
	}
	return Log{
		Path: it.Transcript,
		Ended: func() bool {
			_, ok, err := l.Herdr(ctx, a.String())
			return err == nil && !ok
		},
	}, nil
}
