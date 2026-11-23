package claudeui

import (
	"errors"
	"strings"

	"github.com/elentok/gx/agentrunner"
)

// ErrStuckSubmission means the screen showed no change across Retypes full
// resubmissions: the text never reached the agent, so retrying in the same
// session won't help either.
var ErrStuckSubmission = errors.New("submission never reached the pane")

// ErrStartTimeout means the turn never started within the nudge budget or the
// caller's deadline.
var ErrStartTimeout = errors.New("timed out waiting for agent to start")

// ErrNotReadyToType and ErrInputNotEmpty name why a stuck submission gave up,
// for callers that word the cause for a person. Each travels wrapped with
// ErrStuckSubmission.
var (
	ErrNotReadyToType = errors.New("input not ready to type")
	ErrInputNotEmpty  = errors.New("input not empty after clearing")
)

// screenTailLines is how many of the last screen lines an input-not-empty or
// hung-pane error carries, so the run log and pause can show the pane.
const screenTailLines = 12

// HungPaneError means the pane stopped taking keys: the screen did not change
// across the clear rounds and the input still holds gx's own text. It is
// agentrunner.ErrNotDelivered, so callers start a fresh session, and it keeps
// matching ErrStuckSubmission and ErrInputNotEmpty for existing consumers.
type HungPaneError struct {
	Screen string
}

func (e *HungPaneError) Error() string {
	return "pane hung: screen unchanged across clear rounds and the input still holds gx's text; last screen lines:\n" + ScreenTail(e.Screen)
}

func (e *HungPaneError) Unwrap() []error {
	return []error{agentrunner.ErrNotDelivered, ErrStuckSubmission, ErrInputNotEmpty}
}

// VimEditorError means a session launched with editorMode normal still shows a
// vim footer: the override did not take, so gx cannot type into it safely.
type VimEditorError struct {
	Footer string
}

func (e *VimEditorError) Error() string {
	return "claude is in vim editor mode despite the editorMode normal launch override; footer: " + e.Footer
}

// ScreenTail is the last screenTailLines non-blank lines of screen.
func ScreenTail(screen string) string {
	var lines []string
	for l := range strings.SplitSeq(screen, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimRight(l, " "))
		}
	}
	return strings.Join(lines[max(0, len(lines)-screenTailLines):], "\n")
}

// NudgeDecision is what PromptWithNudge did about a submit the agent didn't
// pick up. Its values are written to the run log as is.
type NudgeDecision string

const (
	NudgeEnter   NudgeDecision = "enter"
	NudgeRetype  NudgeDecision = "retype"
	NudgeStuck   NudgeDecision = "stuck"
	NudgeTimeout NudgeDecision = "timeout"
)
