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

// PermissionPrompt is the waitingFor of both tool permission and plan
// approval. Their first option is "Yes", so Enter allows.
const PermissionPrompt = "permission prompt"

// HolderAnswerer answers an agent's permission dialog over a holder. The queue
// (Pty.Answer) and the keys route share it, so both need only a holder, the
// agent dir and the timing parameters.
type HolderAnswerer struct {
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
	// StrictAllow makes a Claude Allow read the screen first and send Enter only
	// when the cursor is on option 1 and option 1 is exactly "Yes". Anything
	// else, unreadable included, is ErrNotApprovable, so Enter can never pick a
	// standing permission. RequestID is matched against that read.
	StrictAllow bool

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

func (h HolderAnswerer) died(err error) error {
	if h.Died == nil {
		return err
	}
	return h.Died(err)
}

func (h HolderAnswerer) exitedErr() error { return h.died(errors.New("holder exited")) }

// sleep waits one poll, failing once the holder is gone.
func (h HolderAnswerer) sleep() error {
	select {
	case <-h.Exited:
		return h.exitedErr()
	case <-time.After(h.Poll):
		return nil
	}
}

// WaitTurn waits up to timeout for the holder's Turn to pass turn. cancelled,
// when set, is checked before every look.
func (h HolderAnswerer) WaitTurn(turn int, timeout time.Duration, cancelled func() error) (bool, error) {
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
func (h HolderAnswerer) AnswerClaude(a agentrunner.Answer) error {
	if h.StrictAllow {
		return h.answerClaudeStrict(a)
	}
	claude, turn, err := h.Holder.Status()
	if err != nil {
		return h.died(err)
	}
	if claude == nil || claude.Status != harness.ClaudeWaiting {
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
		if claude.WaitingFor != PermissionPrompt {
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
	moved, err := h.WaitTurn(turn, h.Grace, nil)
	if err != nil {
		return err
	}
	if !moved {
		return fmt.Errorf("%w: %s left the dialog open", agentrunner.ErrNotDelivered, a.Decision)
	}
	return nil
}

// answerClaudeStrict is AnswerClaude for StrictAllow. It takes the block from
// either status source, because a harness without a pid file reports through
// its watcher. The screen is the authority: a RequestID that is not the dialog
// on it is ErrRequestMismatch for either decision, and Allow needs the cursor
// on a plain "Yes" first option. Deny is Esc whatever the screen shows.
func (h HolderAnswerer) answerClaudeStrict(a agentrunner.Answer) error {
	if a.Decision != agentrunner.DecisionAllow && a.Decision != agentrunner.DecisionDeny {
		return fmt.Errorf("%w: %q", agentrunner.ErrInvalidDecision, a.Decision)
	}
	resp, err := h.Holder.StatusReply()
	if err != nil {
		return h.died(err)
	}
	w := resp.WatchedView()
	blocked := resp.Claude != nil && resp.Claude.Status == harness.ClaudeWaiting
	if w != nil {
		blocked = w.Status.State == harness.Blocked && w.Status.Blocked == harness.BlockedPermission
	}
	if !blocked {
		return fmt.Errorf("%w: answer on a session that is not blocked", agentrunner.ErrNotReady)
	}
	rows, _, err := h.Holder.Screen(0)
	if err != nil {
		return h.died(err)
	}
	v := claudeui.ReadDialog(rows)
	if a.RequestID != "" && v.ID != a.RequestID {
		return fmt.Errorf("%w: %s", agentrunner.ErrRequestMismatch, a.RequestID)
	}
	key := "\x1b"
	if a.Decision == agentrunner.DecisionAllow {
		if v.Kind != claudeui.DialogPermission || v.Cursor != 0 || !v.PlainYes {
			return fmt.Errorf("%w: %s", ErrNotApprovable, a.Decision)
		}
		key = "\r"
	}
	if err := h.died(h.Holder.SendKeys(key)); err != nil {
		return err
	}
	if w != nil {
		return h.awaitNotBlocked(a.Decision)
	}
	moved, err := h.WaitTurn(resp.Turn, h.Grace, nil)
	if err != nil {
		return err
	}
	if !moved {
		return fmt.Errorf("%w: %s left the dialog open", agentrunner.ErrNotDelivered, a.Decision)
	}
	return nil
}

// AnswerSessionPermission answers the permission dialog of a gx session for
// the keys route: the shared answer logic with the strict Claude Allow and the
// Codex policy for an unreadable screen.
func AnswerSessionPermission(c *HolderClient, dir string, kind agentrunner.Kind, a agentrunner.Answer, poll, grace time.Duration) error {
	h := HolderAnswerer{Holder: c, Dir: dir, Poll: poll, Grace: grace, UnreadableDeny: true, StrictAllow: true}
	if kind == harness.Codex {
		return h.AnswerCodex(a)
	}
	return h.AnswerClaude(a)
}

// denyWithReason prefers the holder's deny-reason op, which keeps Status
// blocked until claude takes the reason; between a bare Esc and the reason,
// a waiter would see idle and end the turn. A failed step leaves claude
// somewhere between the dialog and the reason, so the session is marked died.
func (h HolderAnswerer) denyWithReason(reason string) error {
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
func (h HolderAnswerer) escThenPrompt(reason string) error {
	if err := h.died(h.Holder.SendKeys("\x1b")); err != nil {
		return err
	}
	deadline := time.Now().Add(h.Grace)
	for {
		claude, _, err := h.Holder.Status()
		if err != nil {
			return h.died(err)
		}
		if claude != nil && claude.Status == harness.ClaudeIdle {
			return h.prompt(reason)
		}
		if time.Now().After(deadline) {
			return agentrunner.SessionDied("claude did not go idle after Esc")
		}
		time.Sleep(h.Poll)
	}
}

func (h HolderAnswerer) prompt(text string) error {
	if h.Prompt == nil {
		return nil
	}
	return h.Prompt(text)
}

// codexBlockedOnPermission reports whether Codex is blocked on an approval.
func (h HolderAnswerer) codexBlockedOnPermission() (bool, error) {
	resp, err := h.Holder.StatusReply()
	if err != nil {
		return false, h.died(err)
	}
	w := resp.WatchedView()
	return w != nil && w.Status.State == harness.Blocked && w.Status.Blocked == harness.BlockedPermission, nil
}

func (h HolderAnswerer) CodexScreenDialog() (claudeui.Dialog, error) {
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
func (h HolderAnswerer) AnswerCodex(a agentrunner.Answer) error {
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

func (h HolderAnswerer) answerCodexByKeys(a agentrunner.Answer) error {
	d, err := h.CodexScreenDialog()
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
func (h HolderAnswerer) awaitNotBlocked(decision agentrunner.Decision) error {
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
