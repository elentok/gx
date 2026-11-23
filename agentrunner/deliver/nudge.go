package deliver

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
)

// NudgeCaps bound PromptWithNudge. Grace bounds each submit/nudge wait for the
// turn to start. Nudges caps the retries after the first submit; Retypes caps
// how many of them may resubmit the text in full before PromptWithNudge gives
// up with ErrStuckSubmission. Settle, when set, is a short first wait: if the
// turn has not started by then and the screen shows the text landed (it
// changed), the Enter is pressed at once instead of after the whole Grace.
type NudgeCaps struct {
	Grace   time.Duration
	Nudges  int
	Retypes int
	Settle  time.Duration
}

// DefaultNudgeCaps are the nudge caps. A tighter grace was too short for a
// cold-started agent to reach "working" under concurrent load, even with
// plenty left on the caller's own deadline.
var DefaultNudgeCaps = NudgeCaps{Grace: 45 * time.Second, Nudges: 3, Retypes: 2}

// ReportDecisions adapts agentrunner.Nudger's onNudge to PromptWithNudge's
// report; nil stays nil.
func ReportDecisions(onNudge func(string)) func(claudeui.NudgeDecision) {
	if onNudge == nil {
		return nil
	}
	return func(d claudeui.NudgeDecision) { onNudge(string(d)) }
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

// inputSettleWait bounds how long clearWhole rereads a not-empty input after
// the clear keys, polling every inputSettlePoll.
var (
	inputSettleWait = time.Second
	inputSettlePoll = 25 * time.Millisecond
)

// LogProvider is the optional part of a Prompter that names the logger for
// clearWhole's pane capture; without it the default logger is used.
type LogProvider interface {
	Log() *slog.Logger
}

// clearRounds is how many times clearWhole presses the clear keys before it
// gives up: one more round covers a redraw that lagged past the settle wait.
const clearRounds = 2

// clearWhole empties the whole input, not just its current line, then checks
// it when the Prompter can.
func clearWhole(p Prompter, text string) error {
	c, checks := p.(InputChecker)
	var roundScreens []string
	for range clearRounds {
		for range clearPresses(text) {
			if err := p.Clear(); err != nil {
				return fmt.Errorf("clearing input before retype: %w", err)
			}
		}
		if !checks {
			return nil
		}
		// The agent redraws a moment after the keys land, so a read right away
		// can still show the text.
		settle := time.Now().Add(inputSettleWait)
		empty, known, err := c.InputEmpty()
		for err == nil && known && !empty && time.Now().Before(settle) {
			time.Sleep(inputSettlePoll)
			empty, known, err = c.InputEmpty()
		}
		if err != nil || !known || empty {
			return nil
		}
		s, _ := p.Screen()
		roundScreens = append(roundScreens, s)
	}
	screen, _ := p.Screen()
	log := slog.Default()
	if lp, ok := p.(LogProvider); ok && lp.Log() != nil {
		log = lp.Log()
	}
	log.Info("input not empty after full clear", "screen", screen)
	box, inBox := claudeui.InputBoxRows(strings.Split(screen, "\n"))
	if roundScreens[0] == roundScreens[len(roundScreens)-1] && inBox && boxHoldsOurs(box, text, 0) {
		return &claudeui.HungPaneError{Screen: screen}
	}
	return fmt.Errorf("%w: %w: last screen lines:\n%s", claudeui.ErrStuckSubmission, claudeui.ErrInputNotEmpty, claudeui.ScreenTail(screen))
}

// ClearInput gets the input ready to type, then empties the whole input once,
// as the retype does for text. In vim NORMAL mode the clear keys would run as
// vim commands, so a give-up there is ErrNotReadyToType.
func ClearInput(p Prompter, text string) error {
	if !readyToType(p) {
		return fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrNotReadyToType)
	}
	return clearWhole(p, text)
}

// settleEnterCap caps the Enters one nudge presses while the input still holds
// the text. Claude can swallow two in a row: the first strips a paste's
// invisible characters ("Removed 1 invisible char") and holds it for review.
const settleEnterCap = 3

// nudgeEnter presses Enter, then waits for the turn to start. With settle set
// and a Prompter that can read the input, it looks again after each settle: an
// input still holding the text means the Enter was swallowed, so it presses
// another, up to settleEnterCap, before it waits out the rest of timeout. It
// presses again only when the input box row shows gx's own text.
func nudgeEnter(p Prompter, text string, settle, timeout time.Duration, report func(claudeui.NudgeDecision)) (bool, error) {
	if err := p.SendEnter(); err != nil {
		return false, fmt.Errorf("nudging stuck submission: %w", err)
	}
	if settle <= 0 || settle >= timeout {
		return p.WaitStart(timeout)
	}
	waited := time.Duration(0)
	for presses := 1; ; presses++ {
		started, err := p.WaitStart(settle)
		waited += settle
		if err != nil || started {
			return started, err
		}
		if presses >= settleEnterCap || waited >= timeout {
			break
		}
		screen, readErr := p.Screen()
		box, ok := claudeui.InputBoxRows(strings.Split(screen, "\n"))
		chips := 0
		if ic, has := p.(interface{ ImageCount() int }); has {
			chips = ic.ImageCount()
		}
		if readErr != nil || !ok || !boxHoldsOurs(box, text, chips) {
			break
		}
		report(claudeui.NudgeEnter)
		if err := p.SendEnter(); err != nil {
			return false, fmt.Errorf("nudging stuck submission: %w", err)
		}
	}
	if waited >= timeout {
		return false, nil
	}
	return p.WaitStart(timeout - waited)
}

// PromptWithNudge submits text and waits for the turn to start, working
// around a submission that lands without being sent (seen in production).
// After each attempt that doesn't start, it compares the screen with the one
// before: if it changed, the text likely landed unsent, so it presses a bare
// Enter; if not, the text never arrived, so it clears the input and retypes
// it in full. A zero deadline means none; otherwise every wait is capped to
// what remains of it. report, if not nil, hears every decision.
func PromptWithNudge(p Prompter, text string, caps NudgeCaps, deadline time.Time, now func() time.Time, report func(claudeui.NudgeDecision)) error {
	if report == nil {
		report = func(claudeui.NudgeDecision) {}
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
		report(claudeui.NudgeTimeout)
		return fmt.Errorf("%w: overall deadline exceeded", claudeui.ErrStartTimeout)
	}

	// Before the snapshot: pressing `i` redraws the footer, which would read as
	// a changed screen.
	if !readyToType(p) {
		report(claudeui.NudgeStuck)
		return fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrNotReadyToType)
	}

	// A screen read error counts as "changed": with no evidence the text is
	// missing, a bare Enter is safer than typing it twice.
	snapshot, _ := p.Screen()
	if err := p.Submit(text); err != nil {
		return err
	}
	first := timeout
	if caps.Settle > 0 && caps.Settle < timeout {
		first = caps.Settle
	}
	started, err := p.WaitStart(first)
	if err == nil && !started && first < timeout {
		// Claude swallows an Enter while it reviews a paste: a changed screen
		// means the text is in the input, so the nudge below presses Enter now.
		// An unchanged one may only be slow (an image paste can take seconds to
		// show): keep waiting out the grace, but look again every Settle so a
		// late-landing text gets its Enter then, not at the end of the grace.
		for waited := first; err == nil && !started && waited < timeout; {
			if after, readErr := p.Screen(); readErr != nil || after != snapshot {
				break
			}
			step := min(caps.Settle, timeout-waited)
			started, err = p.WaitStart(step)
			waited += step
		}
	}
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
				report(claudeui.NudgeStuck)
				return fmt.Errorf("%w after %d retries", claudeui.ErrStuckSubmission, retypes)
			}
			retypes++
			report(claudeui.NudgeRetype)
			if !readyToType(p) {
				report(claudeui.NudgeStuck)
				return fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrNotReadyToType)
			}
			// Partial text the screen didn't show would otherwise prefix the
			// retype.
			if err = clearWhole(p, text); err != nil {
				if errors.Is(err, claudeui.ErrStuckSubmission) {
					report(claudeui.NudgeStuck)
				}
				return err
			}
			if err = p.Submit(text); err != nil {
				return err
			}
		} else {
			report(claudeui.NudgeEnter)
			if started, err = nudgeEnter(p, text, caps.Settle, timeout, report); err != nil {
				return err
			}
			continue
		}
		started, err = p.WaitStart(timeout)
	}
	if err != nil {
		return err
	}
	if !started {
		report(claudeui.NudgeTimeout)
		return claudeui.ErrStartTimeout
	}
	return nil
}
