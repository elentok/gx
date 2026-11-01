package nativerunner

import (
	"path/filepath"

	"github.com/elentok/gx/agentrunner"
)

// codexPending is the approval a blocked Codex waits on: the hook's request
// when it fired, else the dialog on the screen. nil when there is neither, or
// the screen is not a dialog gx may answer.
func (p *Pty) codexPending(ss *ptySession) (*agentrunner.Pending, error) {
	req, ok, err := ReadPermissionRequest(p.agentDir(ss))
	if err != nil {
		return nil, err
	}
	if ok {
		return &agentrunner.Pending{ID: req.ID, Tool: req.Tool, Input: req.Input, Description: req.Description}, nil
	}
	d, err := p.answerer(ss).codexScreenDialog()
	if err != nil || !d.Approvable {
		return nil, err
	}
	return &agentrunner.Pending{ID: d.ID, Tool: d.Header, Description: d.Body}, nil
}

func (p *Pty) agentDir(ss *ptySession) string { return filepath.Join(p.Root, ss.session.Label) }

// answerer is the shared answer logic over ss's holder, with the queue's
// timings and its died mapping.
func (p *Pty) answerer(ss *ptySession) holderAnswerer {
	return holderAnswerer{
		Holder:    ss.holder,
		Dir:       p.agentDir(ss),
		Poll:      p.pollInterval(),
		Grace:     p.promptGrace(),
		Died:      func(err error) error { return p.died(ss, err) },
		MarkDied:  func(err error) { p.markDied(ss, err) },
		Exited:    ss.exited,
		Prompt:    func(text string) error { return p.Prompt(ss.session, text) },
		AwaitIdle: func() error { return p.awaitIdle(ss) },
	}
}
