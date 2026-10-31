package nativerunner

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/harness"
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
	d, err := p.codexScreenDialog(ss)
	if err != nil || !d.Approvable {
		return nil, err
	}
	return &agentrunner.Pending{ID: d.ID, Tool: d.Header, Description: d.Body}, nil
}

func (p *Pty) agentDir(ss *ptySession) string { return filepath.Join(p.Root, ss.session.Label) }

func (p *Pty) codexScreenDialog(ss *ptySession) (claudeui.Dialog, error) {
	rows, _, err := ss.holder.Screen(0)
	if err != nil {
		return claudeui.Dialog{}, p.died(ss, err)
	}
	return codexui.ParseDialog(rows), nil
}

// codexBlockedOnPermission reports whether Codex is blocked on an approval.
func (p *Pty) codexBlockedOnPermission(ss *ptySession) (bool, error) {
	resp, err := ss.holder.statusReply()
	if err != nil {
		return false, p.died(ss, err)
	}
	w := resp.watched()
	return w != nil && w.Status.State == harness.Blocked && w.Status.Blocked == harness.BlockedPermission, nil
}

// answerCodex answers Codex's approval. With the hook's request in the agent
// dir the answer is a decision file; without one it is keys on the dialog the
// screen shows, and only a dialog the parser recognises. A RequestID that is
// not the current request is ErrRequestMismatch, before anything is sent.
func (p *Pty) answerCodex(ss *ptySession, a agentrunner.Answer) error {
	blocked, err := p.codexBlockedOnPermission(ss)
	if err != nil {
		return err
	}
	if !blocked {
		return fmt.Errorf("%w: answer on a session that is not blocked", agentrunner.ErrNotReady)
	}
	if a.Decision != agentrunner.DecisionAllow && a.Decision != agentrunner.DecisionDeny {
		return fmt.Errorf("%w: %q", agentrunner.ErrInvalidDecision, a.Decision)
	}
	dir := p.agentDir(ss)
	req, hooked, err := ReadPermissionRequest(dir)
	if err != nil {
		return err
	}
	if hooked {
		if a.RequestID != "" && a.RequestID != req.ID {
			return fmt.Errorf("%w: %s", agentrunner.ErrRequestMismatch, a.RequestID)
		}
		err = WritePermissionDecision(dir, PermissionDecision{ID: req.ID, Allow: a.Decision == agentrunner.DecisionAllow, Reason: a.Text})
	} else {
		err = p.answerCodexByKeys(ss, a)
	}
	if err != nil {
		return err
	}
	return p.awaitNotBlocked(ss, a.Decision)
}

func (p *Pty) answerCodexByKeys(ss *ptySession, a agentrunner.Answer) error {
	d, err := p.codexScreenDialog(ss)
	if err != nil {
		return err
	}
	if !d.Approvable || (a.RequestID != "" && d.ID != a.RequestID) {
		return fmt.Errorf("%w: %s", agentrunner.ErrRequestMismatch, a.RequestID)
	}
	key := codexui.DenyKey
	if a.Decision == agentrunner.DecisionAllow {
		key = codexui.AllowKey
	}
	if err := p.died(ss, ss.holder.SendKeys(key)); err != nil {
		return err
	}
	if a.Decision != agentrunner.DecisionDeny || a.Text == "" {
		return nil
	}
	// The keys only cancel; the reason reaches the model as the next prompt.
	if err := p.awaitIdle(ss); err != nil {
		return err
	}
	return p.Prompt(ss.session, a.Text)
}

// awaitNotBlocked returns once Codex left the approval.
func (p *Pty) awaitNotBlocked(ss *ptySession, decision agentrunner.Decision) error {
	deadline := time.Now().Add(p.promptGrace())
	for {
		blocked, err := p.codexBlockedOnPermission(ss)
		if err != nil || !blocked {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %s left the dialog open", agentrunner.ErrNotDelivered, decision)
		}
		time.Sleep(p.pollInterval())
	}
}
