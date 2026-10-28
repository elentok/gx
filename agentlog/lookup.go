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

	errNoHome = errors.New("home directory unknown")
)

// transcriptPath is where claude keeps the transcript of the session started
// in cwd, "" when the home is unknown.
func (l Locator) transcriptPath(cwd, sessionID string) string {
	if l.Home == "" {
		return ""
	}
	return transcript.PathIn(l.Home, cwd, sessionID)
}

// Log is where one agent's log lives.
type Log struct {
	// Path is the JSONL file to read from Offset: out.jsonl for a headless
	// agent, the claude transcript for a pty or herdr one. Empty when Pruned.
	Path   string
	Offset int64
	// Ended reports whether the agent is gone, so follow knows to stop.
	Ended func() bool
	// Pruned: retention removed the native log. Transcript still names the
	// claude transcript, which claude keeps on its own schedule.
	Pruned     bool
	Transcript string
	// PtyDir is a pty agent's directory, which holds its holder.sock,
	// screen.txt and raw.log. Empty for any other agent.
	PtyDir string
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
	// Home is the user's home, where claude keeps its transcripts. Empty means
	// unknown: a native claude agent then has no transcript path.
	Home string
	// Procs overrides the process table native liveness checks use.
	Procs nativerunner.ProcessTable
	// Herdr looks up a live herdr iteration by full ticket address. Nil
	// means herdr isn't in play.
	Herdr func(ctx context.Context, address string) (HerdrIteration, bool, error)
}

// AgentLog finds the log of a's agent.
func (l Locator) AgentLog(ctx context.Context, a tickets.Address) (Log, error) {
	label, _, _ := ralphloop.IterationIdentity(a.Epic, a.ID, "")
	path := filepath.Join(nativerunner.AgentsRoot(l.StateDir, a.Project), label)
	d, err := nativerunner.OpenDir(path)
	switch {
	case err == nil:
		var ptyDir string
		if d.PTY() {
			ptyDir = path
		}
		if d.Pruned() {
			return Log{Pruned: true, Transcript: l.transcriptPath(d.Meta.Cwd, d.Meta.SessionID), PtyDir: ptyDir}, nil
		}
		ended := func() bool {
			d, err := nativerunner.OpenDir(path)
			if err != nil {
				return false
			}
			live, err := d.Alive(l.Procs)
			return err == nil && !live
		}
		if d.ReadsTranscript() {
			if l.Home == "" {
				return Log{}, errNoHome
			}
			p := l.transcriptPath(d.Meta.Cwd, d.Meta.SessionID)
			return Log{Path: transcript.Resolve(p), Ended: ended, PtyDir: ptyDir}, nil
		}
		return Log{Path: d.Log(), Offset: d.Meta.Offset, Ended: ended}, nil
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
		Path: transcript.Resolve(it.Transcript),
		Ended: func() bool {
			_, ok, err := l.Herdr(ctx, a.String())
			return err == nil && !ok
		},
	}, nil
}
