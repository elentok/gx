package nativerunner

import (
	"errors"
	"fmt"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/harness"
)

// ErrNotApprovable is an Allow on a Codex dialog gx cannot read: Esc still
// cancels it, but nothing says which option approves.
var ErrNotApprovable = errors.New("dialog is not approvable")

// holderAnswerer answers an agent's permission dialog over a holder. The queue
// (Pty.Answer) and the keys route share it, so both need only a holder, the
// agent dir and the timing parameters.
type holderAnswerer struct {
	Holder *HolderClient
	// Dir is the agent dir the Codex hook writes its request and decision in.
	Dir  string
	Poll time.Duration
	// Grace is how long an answer waits for the agent to move on.
	Grace time.Duration
	// UnreadableDeny sets the Codex policy for a screen gx cannot read and no
	// hook request: Deny sends Esc, Allow is ErrNotApprovable. Off, both are
	// ErrRequestMismatch.
	UnreadableDeny bool

	// Died maps a failed holder call; nil keeps the error.
	Died func(error) error
	// MarkDied records a session that is lost mid deny-reason; nil ignores it.
	MarkDied func(error)
	// Exited, when set, closes once the holder is gone.
	Exited <-chan struct{}
	// Prompt types text as the agent's next prompt, and AwaitIdle waits for the
	// agent to take one. A deny reason travels this way where the holder has no
	// deny-reason op (Claude) or the keys only cancel (Codex). Nil Prompt drops
	// the reason.
	Prompt    func(text string) error
	AwaitIdle func() error
}

func (h holderAnswerer) died(err error) error {
	if h.Died == nil {
		return err
	}
	return h.Died(err)
}

func (h holderAnswerer) exitedErr() error { return h.died(errors.New("holder exited")) }

// sleep waits one poll, failing once the holder is gone.
func (h holderAnswerer) sleep() error {
	select {
	case <-h.Exited:
		return h.exitedErr()
	case <-time.After(h.Poll):
		return nil
	}
}

// waitTurn waits up to timeout for the holder's Turn to pass turn. cancelled,
// when set, is checked before every look.
func (h holderAnswerer) waitTurn(turn int, timeout time.Duration, cancelled func() error) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		if cancelled != nil {
			if err := cancelled(); err != nil {
				return false, err
			}
		}
		_, now, err := h.Holder.Status()
		if err != nil {
			return false, h.died(err)
		}
		if now > turn {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		if err := h.sleep(); err != nil {
			return false, err
		}
	}
}

// AnswerClaude presses Enter to allow a permission prompt or Esc to deny any
// dialog, and returns once Turn moved. Allow on any other dialog is
// ErrInvalidDecision: its options aren't known, but Esc always cancels it.
// Deny with a reason also sends the reason as claude's next prompt. With a
// RequestID, the screen is re-read first and anything but that approvable
// dialog is ErrRequestMismatch, before any key is sent.
func (h holderAnswerer) AnswerClaude(a agentrunner.Answer) error {
	claude, turn, err := h.Holder.Status()
	if err != nil {
		return h.died(err)
	}
	if claude == nil || claude.Status != ClaudeWaiting {
		return fmt.Errorf("%w: answer on a session that is not blocked", agentrunner.ErrNotReady)
	}
	if a.RequestID != "" {
		rows, _, err := h.Holder.Screen(0)
		if err != nil {
			return h.died(err)
		}
		if d := claudeui.ParseDialog(rows); !d.Approvable || d.ID != a.RequestID {
			return fmt.Errorf("%w: %s", agentrunner.ErrRequestMismatch, a.RequestID)
		}
	}
	var key string
	switch a.Decision {
	case agentrunner.DecisionAllow:
		if claude.WaitingFor != permissionPrompt {
			return fmt.Errorf("%w: %q is not a permission prompt; deny cancels it",
				agentrunner.ErrInvalidDecision, claude.WaitingFor)
		}
		key = "\r"
	case agentrunner.DecisionDeny:
		if a.Text != "" {
			return h.denyWithReason(a.Text)
		}
		key = "\x1b"
	default:
		return fmt.Errorf("%w: %q", agentrunner.ErrInvalidDecision, a.Decision)
	}
	if err := h.died(h.Holder.SendKeys(key)); err != nil {
		return err
	}
	moved, err := h.waitTurn(turn, h.Grace, nil)
	if err != nil {
		return err
	}
	if !moved {
		return fmt.Errorf("%w: %s left the dialog open", agentrunner.ErrNotDelivered, a.Decision)
	}
	return nil
}

// denyWithReason prefers the holder's deny-reason op, which keeps Status
// blocked until claude takes the reason; between a bare Esc and the reason,
// a waiter would see idle and end the turn. A failed step leaves claude
// somewhere between the dialog and the reason, so the session is marked died.
func (h holderAnswerer) denyWithReason(reason string) error {
	var err error
	if h.Holder.Supports(OpDenyReason) {
		err = h.died(h.Holder.DenyWithReason(reason))
	} else {
		err = h.escThenPrompt(reason)
	}
	if errors.Is(err, agentrunner.ErrSessionDied) && h.MarkDied != nil {
		h.MarkDied(err)
	}
	return err
}

// escThenPrompt is deny-reason over core ops, for a holder that predates the
// op: Esc, then the reason as a normal prompt once claude is idle.
func (h holderAnswerer) escThenPrompt(reason string) error {
	if err := h.died(h.Holder.SendKeys("\x1b")); err != nil {
		return err
	}
	deadline := time.Now().Add(h.Grace)
	for {
		claude, _, err := h.Holder.Status()
		if err != nil {
			return h.died(err)
		}
		if claude != nil && claude.Status == ClaudeIdle {
			return h.prompt(reason)
		}
		if time.Now().After(deadline) {
			return agentrunner.SessionDied("claude did not go idle after Esc")
		}
		time.Sleep(h.Poll)
	}
}

func (h holderAnswerer) prompt(text string) error {
	if h.Prompt == nil {
		return nil
	}
	return h.Prompt(text)
}

// codexBlockedOnPermission reports whether Codex is blocked on an approval.
func (h holderAnswerer) codexBlockedOnPermission() (bool, error) {
	resp, err := h.Holder.statusReply()
	if err != nil {
		return false, h.died(err)
	}
	w := resp.watched()
	return w != nil && w.Status.State == harness.Blocked && w.Status.Blocked == harness.BlockedPermission, nil
}

func (h holderAnswerer) codexScreenDialog() (claudeui.Dialog, error) {
	rows, _, err := h.Holder.Screen(0)
	if err != nil {
		return claudeui.Dialog{}, h.died(err)
	}
	return codexui.ParseDialog(rows), nil
}

// AnswerCodex answers Codex's approval. With the hook's request in the agent
// dir the answer is a decision file; without one it is keys on the dialog the
// screen shows, and only a dialog the parser recognises. A RequestID that is
// not the current request is ErrRequestMismatch, before anything is sent.
func (h holderAnswerer) AnswerCodex(a agentrunner.Answer) error {
	blocked, err := h.codexBlockedOnPermission()
	if err != nil {
		return err
	}
	if !blocked {
		return fmt.Errorf("%w: answer on a session that is not blocked", agentrunner.ErrNotReady)
	}
	if a.Decision != agentrunner.DecisionAllow && a.Decision != agentrunner.DecisionDeny {
		return fmt.Errorf("%w: %q", agentrunner.ErrInvalidDecision, a.Decision)
	}
	req, hooked, err := ReadPermissionRequest(h.Dir)
	if err != nil {
		return err
	}
	if hooked {
		if a.RequestID != "" && a.RequestID != req.ID {
			return fmt.Errorf("%w: %s", agentrunner.ErrRequestMismatch, a.RequestID)
		}
		err = WritePermissionDecision(h.Dir, PermissionDecision{ID: req.ID, Allow: a.Decision == agentrunner.DecisionAllow, Reason: a.Text})
	} else {
		err = h.answerCodexByKeys(a)
	}
	if err != nil {
		return err
	}
	return h.awaitNotBlocked(a.Decision)
}

func (h holderAnswerer) answerCodexByKeys(a agentrunner.Answer) error {
	d, err := h.codexScreenDialog()
	if err != nil {
		return err
	}
	switch {
	case !d.Approvable && h.UnreadableDeny:
		if a.Decision != agentrunner.DecisionDeny {
			return fmt.Errorf("%w: %s", ErrNotApprovable, a.Decision)
		}
	case !d.Approvable, a.RequestID != "" && d.ID != a.RequestID:
		return fmt.Errorf("%w: %s", agentrunner.ErrRequestMismatch, a.RequestID)
	}
	key := codexui.DenyKey
	if a.Decision == agentrunner.DecisionAllow {
		key = codexui.AllowKey
	}
	if err := h.died(h.Holder.SendKeys(key)); err != nil {
		return err
	}
	if a.Decision != agentrunner.DecisionDeny || a.Text == "" {
		return nil
	}
	// The keys only cancel; the reason reaches the model as the next prompt.
	if h.AwaitIdle != nil {
		if err := h.AwaitIdle(); err != nil {
			return err
		}
	}
	return h.prompt(a.Text)
}

// awaitNotBlocked returns once Codex left the approval.
func (h holderAnswerer) awaitNotBlocked(decision agentrunner.Decision) error {
	deadline := time.Now().Add(h.Grace)
	for {
		blocked, err := h.codexBlockedOnPermission()
		if err != nil || !blocked {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %s left the dialog open", agentrunner.ErrNotDelivered, decision)
		}
		time.Sleep(h.Poll)
	}
}
