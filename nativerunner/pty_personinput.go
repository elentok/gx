package nativerunner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
)

var (
	_ agentrunner.PersonInputter = (*Pty)(nil)

	// errPersonInput ends a prompt once a person typed after gx did: the rest of
	// its nudges would fight that input. It is not a failure.
	errPersonInput = errors.New("a person typed after the prompt")
)

// PersonInput watches the holder for a person's keys and pastes. A holder
// without the watch op returns an error.
func (p *Pty) PersonInput(s agentrunner.Session) (<-chan struct{}, func(), error) {
	ss, err := p.reg.live(s)
	if err != nil {
		return nil, nil, err
	}
	if !ss.holder.Supports(holder.OpWatch) {
		return nil, nil, errors.New("holder has no watch op")
	}
	watch, err := ss.holder.Watch()
	if err != nil {
		return nil, nil, err
	}
	events := make(chan struct{}, 1)
	go func() {
		for {
			m, err := watch.Next()
			if err != nil {
				return
			}
			if m.Type != holder.StreamInput {
				continue
			}
			select {
			case events <- struct{}{}:
			default:
			}
		}
	}()
	return events, func() { _ = watch.Close() }, nil
}

// ptyPrompter is claudeui.Prompter over holder.sock. A turn has started once
// the holder's Turn passes turn.
//
// person carries a person's input (nil when unseen). Input before gx typed is
// the draft check's case; input after it sets stopped, and every later key or
// wait returns errPersonInput.
type ptyPrompter struct {
	p *Pty
	// ctx ends the launch prompt's waits when the server stops; nil never ends.
	ctx     context.Context
	ss      *ptySession
	turn    int
	person  <-chan struct{}
	typed   bool
	stopped bool
}

// cancelled returns the launch ctx's error once it ends, and errPersonInput
// once a person typed after gx did.
func (pr *ptyPrompter) cancelled() error {
	if pr.ctx != nil {
		if err := pr.ctx.Err(); err != nil {
			return fmt.Errorf("launch prompt cancelled: %w", context.Cause(pr.ctx))
		}
	}
	if !pr.typed {
		return nil
	}
	if !pr.stopped {
		select {
		case <-pr.person:
			pr.stopped = true
		default:
		}
	}
	if pr.stopped {
		return errPersonInput
	}
	return nil
}

// inputNotEmpty is claudeui.ErrInputNotEmpty with wording that claims no clear:
// most drafts are a person's and stay.
type inputNotEmpty struct{}

func (inputNotEmpty) Error() string { return "input not empty, only a lone i that gx typed is cleared" }

func (inputNotEmpty) Is(target error) bool { return target == claudeui.ErrInputNotEmpty }

// clearGxI presses the clear key when the input is exactly `i` and the probe
// pressed an `i` since the last send. Any other content, a one-letter draft
// included, stays. It reports whether it cleared.
func (pr *ptyPrompter) clearGxI() (bool, error) {
	if !pr.ss.holder.Supports(holder.OpScreenStyled) {
		return false, nil
	}
	rows, err := pr.ss.holder.StyledScreen()
	if err != nil {
		return false, pr.p.died(pr.ss, err)
	}
	t := pr.p.typing(pr.ss)
	if text, ok := harness.InputText(rows); !ok || text != "i" || int64(t.InputPresses(pr.ss.session.SessionID)) <= pr.ss.sentPresses.Load() {
		return false, nil
	}
	return true, pr.p.died(pr.ss, pr.ss.holder.SendKeys(t.ClearKey()))
}

// awaitEmptyInput waits up to wait for a person's draft to go, so gx never
// types over it. It first clears a lone `i` gx typed, and does so once more
// before giving up. A check that can't decide types as before.
func (pr *ptyPrompter) awaitEmptyInput(wait time.Duration) error {
	if _, err := pr.clearGxI(); err != nil {
		return err
	}
	deadline := time.Now().Add(wait)
	extraRound := false
	for {
		if pr.ctx != nil && pr.ctx.Err() != nil {
			return pr.cancelled()
		}
		empty, known, err := pr.InputEmpty()
		if err != nil || empty || !known {
			return err
		}
		if !time.Now().Before(deadline) {
			if extraRound {
				return fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, inputNotEmpty{})
			}
			extraRound = true
			cleared, err := pr.clearGxI()
			if err != nil {
				return err
			}
			if pr.p.log != nil {
				screen, _ := pr.Screen()
				pr.p.log.Info("input not empty, extra clear round", "session", pr.ss.session.SessionID, "cleared", cleared, "screen", screen)
			}
			deadline = time.Now().Add(5 * pr.p.pollInterval())
			continue
		}
		select {
		case <-pr.ss.exited:
			return pr.p.died(pr.ss, errors.New("holder exited"))
		case <-time.After(pr.p.pollInterval()):
		}
	}
}

func (pr *ptyPrompter) Submit(text string) error {
	if err := pr.cancelled(); err != nil {
		return err
	}
	if !pr.typed {
		// Input so far was before gx typed.
		select {
		case <-pr.person:
		default:
		}
	}
	if err := pr.ss.holder.TypeTextBy(text, pr.p.typing(pr.ss).PastesText(text)); err != nil {
		return pr.p.died(pr.ss, err)
	}
	pr.typed = true
	pr.ss.sentPresses.Store(int64(pr.p.typing(pr.ss).InputPresses(pr.ss.session.SessionID)))
	return pr.SendEnter()
}

// Clear presses the harness clear key once.
func (pr *ptyPrompter) Clear() error {
	if err := pr.cancelled(); err != nil {
		return err
	}
	return pr.p.died(pr.ss, pr.ss.holder.SendKeys(pr.p.typing(pr.ss).ClearKey()))
}

// InputEmpty runs the harness empty check when the holder serves styled rows.
func (pr *ptyPrompter) InputEmpty() (bool, bool, error) {
	if !pr.ss.holder.Supports(holder.OpScreenStyled) {
		return false, false, nil
	}
	rows, err := pr.ss.holder.StyledScreen()
	if err != nil {
		return false, false, pr.p.died(pr.ss, err)
	}
	switch pr.p.typing(pr.ss).InputIsEmpty(rows) {
	case harness.InputEmpty:
		return true, true, nil
	case harness.InputNotEmpty:
		return false, true, nil
	}
	return false, false, nil
}

// ReadyToType runs claude's ready-to-type step over the holder.
func (pr *ptyPrompter) ReadyToType() bool {
	io := HolderTypingIO(pr.ss.holder)
	io.Session = pr.ss.session.SessionID
	io.Log = pr.p.log
	io.Caller = "pty-nudge"
	return pr.p.typing(pr.ss).ReadyToType(io)
}

// HolderTypingIO reads styled rows when the holder serves them, plain rows
// otherwise; the ready-to-type step needs only the text.
func HolderTypingIO(h *holder.HolderClient) harness.TypingIO {
	return harness.TypingIO{
		Rows: func() ([]string, error) {
			if h.Supports(holder.OpScreenStyled) {
				return h.StyledScreen()
			}
			rows, _, err := h.Screen(0)
			return rows, err
		},
		SendKeys: h.SendKeys,
	}
}

// typing is the typing rules of ss's harness. Claude reads its config from
// p.Env; a nil Env becomes an empty one, which harness would read as the
// process env.
func (p *Pty) typing(ss *ptySession) harness.Typing {
	if ss.kind != "" && ss.kind != harness.Claude {
		if t, err := harness.LookupTyping(ss.kind); err == nil {
			return t
		}
	}
	env := p.Env
	if env == nil {
		env = []string{}
	}
	return harness.ClaudeTypingIn(env)
}

// SendEnter presses the harness submit key. Codex queues text with Tab while a
// turn runs.
func (pr *ptyPrompter) SendEnter() error {
	if err := pr.cancelled(); err != nil {
		return err
	}
	working := false
	if w, err := pr.ss.holder.Watched(); err != nil {
		return pr.p.died(pr.ss, err)
	} else if w != nil {
		// A background terminal alone is no turn: Enter sends.
		working = w.Status.State == harness.Working && !w.Status.Background
	}
	return pr.p.died(pr.ss, pr.ss.holder.SendKeys(pr.p.typing(pr.ss).SubmitKey(working)))
}

func (pr *ptyPrompter) WaitStart(timeout time.Duration) (bool, error) {
	return pr.p.answerer(pr.ss).WaitTurn(pr.turn, timeout, pr.cancelled)
}

func (pr *ptyPrompter) Screen() (string, error) {
	rows, _, err := pr.ss.holder.Screen(0)
	return strings.Join(rows, "\n"), pr.p.died(pr.ss, err)
}
