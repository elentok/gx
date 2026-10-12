package claudeui

import (
	"errors"
	"fmt"
	"time"
)

// NudgeGrace bounds each submit/nudge wait for the turn to start. MaxNudges
// caps the retries after the first submit; MaxRetypes caps how many of them
// may resubmit the text in full before PromptWithNudge gives up with
// ErrStuckSubmission.
//
// A tighter grace was too short for a cold-started agent to reach "working"
// under concurrent load, even with plenty left on the caller's own deadline.
const (
	NudgeGrace = 45 * time.Second
	MaxNudges  = 3
	MaxRetypes = 2
)

// ErrStuckSubmission means the screen showed no change across MaxRetypes full
// resubmissions: the text never reached the agent, so retrying in the same
// session won't help either.
var ErrStuckSubmission = errors.New("submission never reached the pane")

// ErrStartTimeout means the turn never started within the nudge budget or the
// caller's deadline.
var ErrStartTimeout = errors.New("timed out waiting for agent to start")

// Prompter is what PromptWithNudge needs from a runner. Submit types text and
// presses Enter. WaitStart reports false, not an error, when timeout elapses
// with no turn started; an error aborts the prompt.
type Prompter interface {
	Submit(text string) error
	SendEnter() error
	WaitStart(timeout time.Duration) (bool, error)
	Screen() (string, error)
}

// PromptWithNudge submits text and waits for the turn to start, working
// around a submission that lands without being sent (seen in production).
// After each attempt that doesn't start, it compares the screen with the one
// before: if it changed, the text likely landed unsent, so it presses a bare
// Enter; if not, the text never arrived, so it retypes it in full. A zero
// deadline means none; otherwise every wait is capped to what remains of it.
func PromptWithNudge(p Prompter, text string, deadline time.Time, now func() time.Time) error {
	grace := func() (time.Duration, bool) {
		if deadline.IsZero() {
			return NudgeGrace, true
		}
		remaining := deadline.Sub(now())
		return min(remaining, NudgeGrace), remaining > 0
	}

	timeout, ok := grace()
	if !ok {
		return fmt.Errorf("%w: overall deadline exceeded", ErrStartTimeout)
	}

	// A screen read error counts as "changed": with no evidence the text is
	// missing, a bare Enter is safer than typing it twice.
	snapshot, _ := p.Screen()
	if err := p.Submit(text); err != nil {
		return err
	}
	started, err := p.WaitStart(timeout)
	retypes := 0
	for attempt := 0; err == nil && !started && attempt < MaxNudges; attempt++ {
		if timeout, ok = grace(); !ok {
			break
		}

		after, readErr := p.Screen()
		unchanged := readErr == nil && after == snapshot
		snapshot = after

		if unchanged {
			if retypes >= MaxRetypes {
				return fmt.Errorf("%w after %d retries", ErrStuckSubmission, retypes)
			}
			retypes++
			if err = p.Submit(text); err != nil {
				return err
			}
		} else if err = p.SendEnter(); err != nil {
			return fmt.Errorf("nudging stuck submission: %w", err)
		}
		started, err = p.WaitStart(timeout)
	}
	if err != nil {
		return err
	}
	if !started {
		return ErrStartTimeout
	}
	return nil
}
