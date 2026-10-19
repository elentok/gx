package claudeui

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// NudgeCaps bound PromptWithNudge. Grace bounds each submit/nudge wait for the
// turn to start. Nudges caps the retries after the first submit; Retypes caps
// how many of them may resubmit the text in full before PromptWithNudge gives
// up with ErrStuckSubmission.
type NudgeCaps struct {
	Grace   time.Duration
	Nudges  int
	Retypes int
}

// DefaultNudgeCaps are herdr's caps. A tighter grace was too short for a
// cold-started agent to reach "working" under concurrent load, even with
// plenty left on the caller's own deadline.
var DefaultNudgeCaps = NudgeCaps{Grace: 45 * time.Second, Nudges: 3, Retypes: 2}

// ErrStuckSubmission means the screen showed no change across Retypes full
// resubmissions: the text never reached the agent, so retrying in the same
// session won't help either.
var ErrStuckSubmission = errors.New("submission never reached the pane")

// ErrStartTimeout means the turn never started within the nudge budget or the
// caller's deadline.
var ErrStartTimeout = errors.New("timed out waiting for agent to start")

// NudgeDecision is what PromptWithNudge did about a submit the agent didn't
// pick up. Its values are written to the run log as is.
type NudgeDecision string

const (
	NudgeEnter   NudgeDecision = "enter"
	NudgeRetype  NudgeDecision = "retype"
	NudgeStuck   NudgeDecision = "stuck"
	NudgeTimeout NudgeDecision = "timeout"
)

// ReportDecisions adapts agentrunner.Nudger's onNudge to PromptWithNudge's
// report; nil stays nil.
func ReportDecisions(onNudge func(string)) func(NudgeDecision) {
	if onNudge == nil {
		return nil
	}
	return func(d NudgeDecision) { onNudge(string(d)) }
}

// Prompter is what PromptWithNudge needs from a runner. Submit types text and
// presses Enter. Clear presses the harness clear key once. WaitStart reports false, not an
// error, when timeout elapses with no turn started; an error aborts the prompt.
type Prompter interface {
	Submit(text string) error
	Clear() error
	SendEnter() error
	WaitStart(timeout time.Duration) (bool, error)
	Screen() (string, error)
}

// InputChecker is the optional part of a Prompter that can tell whether the
// input box is empty from a styled screen read. known is false when the read
// carries no styling, so the clear count alone decides.
type InputChecker interface {
	InputEmpty() (empty, known bool, err error)
}

// ReadyChecker is the optional part of a Prompter that runs the harness's
// ready-to-type step. PromptWithNudge calls it before typing and before the
// retype's clear; false means give-up, which stops the nudge as stuck.
type ReadyChecker interface {
	ReadyToType() bool
}

// readyToType is true for a Prompter with no ready-to-type step.
func readyToType(p Prompter) bool {
	r, ok := p.(ReadyChecker)
	return !ok || r.ReadyToType()
}

// clearPresses is how many clear-key presses empty an input holding text: one
// per line to delete it and one per newline between lines (2N-1), plus two
// spare for a stray trailing line.
func clearPresses(text string) int {
	n := strings.Count(text, "\n") + 1
	return 2*n - 1 + 2
}

// clearWhole empties the whole input, not just its current line, then checks
// it when the Prompter can.
func clearWhole(p Prompter, text string) error {
	for range clearPresses(text) {
		if err := p.Clear(); err != nil {
			return fmt.Errorf("clearing input before retype: %w", err)
		}
	}
	c, ok := p.(InputChecker)
	if !ok {
		return nil
	}
	empty, known, err := c.InputEmpty()
	if err != nil || !known || empty {
		return nil
	}
	screen, _ := p.Screen()
	slog.Debug("input not empty after full clear", "screen", screen)
	return fmt.Errorf("%w: input not empty after clearing", ErrStuckSubmission)
}

// PromptWithNudge submits text and waits for the turn to start, working
// around a submission that lands without being sent (seen in production).
// After each attempt that doesn't start, it compares the screen with the one
// before: if it changed, the text likely landed unsent, so it presses a bare
// Enter; if not, the text never arrived, so it clears the input and retypes
// it in full. A zero deadline means none; otherwise every wait is capped to
// what remains of it. report, if not nil, hears every decision.
func PromptWithNudge(p Prompter, text string, caps NudgeCaps, deadline time.Time, now func() time.Time, report func(NudgeDecision)) error {
	if report == nil {
		report = func(NudgeDecision) {}
	}
	grace := func() (time.Duration, bool) {
		if deadline.IsZero() {
			return caps.Grace, true
		}
		remaining := deadline.Sub(now())
		return min(remaining, caps.Grace), remaining > 0
	}

	timeout, ok := grace()
	if !ok {
		report(NudgeTimeout)
		return fmt.Errorf("%w: overall deadline exceeded", ErrStartTimeout)
	}

	// Before the snapshot: pressing `i` redraws the footer, which would read as
	// a changed screen.
	if !readyToType(p) {
		report(NudgeStuck)
		return fmt.Errorf("%w: input not ready to type", ErrStuckSubmission)
	}

	// A screen read error counts as "changed": with no evidence the text is
	// missing, a bare Enter is safer than typing it twice.
	snapshot, _ := p.Screen()
	if err := p.Submit(text); err != nil {
		return err
	}
	started, err := p.WaitStart(timeout)
	retypes := 0
	for attempt := 0; err == nil && !started && attempt < caps.Nudges; attempt++ {
		if timeout, ok = grace(); !ok {
			break
		}

		after, readErr := p.Screen()
		unchanged := readErr == nil && after == snapshot
		snapshot = after

		if unchanged {
			if retypes >= caps.Retypes {
				report(NudgeStuck)
				return fmt.Errorf("%w after %d retries", ErrStuckSubmission, retypes)
			}
			retypes++
			report(NudgeRetype)
			if !readyToType(p) {
				report(NudgeStuck)
				return fmt.Errorf("%w: input not ready to type", ErrStuckSubmission)
			}
			// Partial text the screen didn't show would otherwise prefix the
			// retype.
			if err = clearWhole(p, text); err != nil {
				if errors.Is(err, ErrStuckSubmission) {
					report(NudgeStuck)
				}
				return err
			}
			if err = p.Submit(text); err != nil {
				return err
			}
		} else {
			report(NudgeEnter)
			if err = p.SendEnter(); err != nil {
				return fmt.Errorf("nudging stuck submission: %w", err)
			}
		}
		started, err = p.WaitStart(timeout)
	}
	if err != nil {
		return err
	}
	if !started {
		report(NudgeTimeout)
		return ErrStartTimeout
	}
	return nil
}
