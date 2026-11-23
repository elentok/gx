package nativerunner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/agentrunner/deliver"
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

// ptyRelay is what a PTY prompt keeps for deliver.Deliver: the cancel check, the
// start signal and the draft wait. Deliver does the typing.
//
// person carries a person's input (nil when unseen). Input before gx typed is
// the draft check's case; input after it sets stopped, and every later key or
// wait returns errPersonInput.
type ptyRelay struct {
	p *Pty
	// ctx ends the launch prompt's waits when the server stops; nil never ends.
	ctx     context.Context
	ss      *ptySession
	turn    int
	person  <-chan struct{}
	typed   bool
	stopped bool
	// cancelErr is the error cancelled last returned, so deliverErr can tell it
	// from a holder failure.
	cancelErr error
}

// cancelled returns the launch ctx's error once it ends, and errPersonInput
// once a person typed after gx did.
func (pr *ptyRelay) cancelled() error {
	err := pr.cancelCheck()
	if err != nil {
		pr.cancelErr = err
	}
	return err
}

func (pr *ptyRelay) cancelCheck() error {
	if pr.ctx != nil {
		if err := pr.ctx.Err(); err != nil {
			return fmt.Errorf("launch prompt cancelled: %w", context.Cause(pr.ctx))
		}
	}
	if !pr.typed {
		// Input so far was before gx typed.
		select {
		case <-pr.person:
		default:
		}
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

// target is the deliver.Target over this session: Deliver reads the submit key
// and paste rule from its Typing, and the start signal and cancel check are ours.
func (pr *ptyRelay) target() *deliver.Target {
	io := HolderTypingIO(pr.ss.holder)
	io.Session = pr.ss.session.SessionID
	io.Log = pr.p.log
	io.Caller = "pty-nudge"
	typing := pr.p.typing(pr.ss)
	return &deliver.Target{
		C:         pr.ss.holder,
		Typing:    typing,
		IO:        io,
		CancelErr: pr.cancelled,
		OnTyped: func() {
			pr.typed = true
			pr.ss.sentPresses.Store(int64(typing.InputPresses(pr.ss.session.SessionID)))
		},
		Start: pr.waitStart,
	}
}

func (pr *ptyRelay) waitStart(timeout time.Duration) (bool, error) {
	return pr.p.answerer(pr.ss).WaitTurn(pr.turn, timeout, pr.cancelled)
}

// deliver sends text through Deliver and maps a holder failure as died does.
func (pr *ptyRelay) deliver(text string, policy deliver.Policy) error {
	return pr.deliverErr(deliver.Deliver(pr.target(), text, policy))
}

// deliverErr maps an error from a holder call to the session's death and leaves
// Deliver's own outcomes as they are.
func (pr *ptyRelay) deliverErr(err error) error {
	if err == nil {
		return nil
	}
	for _, own := range []error{
		errPersonInput, pr.cancelErr, deliver.ErrMaybeUnsent, deliver.ErrCancelled,
		agentrunner.ErrSessionDied, agentrunner.ErrNotDelivered, agentrunner.ErrNotReady,
		agentrunner.ErrUnknownSkill, agentrunner.ErrDeliveryUnconfirmed,
		claudeui.ErrStuckSubmission, claudeui.ErrStartTimeout,
	} {
		if own != nil && errors.Is(err, own) {
			return err
		}
	}
	return pr.p.died(pr.ss, err)
}

// pane is the launch proof's screen read: it stops once the prompt is cancelled.
func (pr *ptyRelay) pane() (string, error) {
	if err := pr.cancelled(); err != nil {
		return "", err
	}
	s, err := pr.target().Screen()
	return s, pr.p.died(pr.ss, err)
}

// batch runs one input step with gx owning the input; waits between steps hold
// nothing, so a person's key lands in them.
func (pr *ptyRelay) batch(fn func() error) error {
	return pr.p.died(pr.ss, pr.ss.holder.Batch(fn))
}

// inputNotEmpty is claudeui.ErrInputNotEmpty with wording that claims no clear:
// most drafts are a person's and stay.
type inputNotEmpty struct{}

func (inputNotEmpty) Error() string { return "input not empty, only a lone i that gx typed is cleared" }

func (inputNotEmpty) Is(target error) bool { return target == claudeui.ErrInputNotEmpty }

// clearGxI presses the clear key when the input is exactly `i` and the probe
// pressed an `i` since the last send. Any other content, a one-letter draft
// included, stays. It reports whether it cleared.
func (pr *ptyRelay) clearGxI() (bool, error) {
	if !pr.ss.holder.Supports(holder.OpScreenStyled) {
		return false, nil
	}
	rows, read, err := pr.ss.holder.StyledScreenRead()
	if err != nil {
		return false, pr.p.died(pr.ss, err)
	}
	if read.Resized {
		return false, nil
	}
	t := pr.p.typing(pr.ss)
	if text, ok := harness.InputText(rows); !ok || text != "i" || int64(t.InputPresses(pr.ss.session.SessionID)) <= pr.ss.sentPresses.Load() {
		return false, nil
	}
	return true, pr.batch(func() error { return pr.ss.holder.SendKeys(t.ClearKey()) })
}

// awaitEmptyInput waits up to wait for a person's draft to go, so gx never
// types over it. It first clears a lone `i` gx typed, and does so once more
// before giving up. A check that can't decide types as before.
func (pr *ptyRelay) awaitEmptyInput(wait time.Duration) error {
	if _, err := pr.clearGxI(); err != nil {
		return err
	}
	tg := pr.target()
	deadline := time.Now().Add(wait)
	extraRound, gxCleared := false, false
	var before string
	for {
		if pr.ctx != nil && pr.ctx.Err() != nil {
			return pr.cancelled()
		}
		empty, known, err := tg.InputEmpty()
		if err != nil || empty || !known {
			return pr.p.died(pr.ss, err)
		}
		if !time.Now().Before(deadline) {
			if extraRound {
				// gx's own `i` survived its clear key and nothing moved on screen.
				if screen, _ := tg.Screen(); gxCleared && screen == before {
					return &claudeui.HungPaneError{Screen: screen}
				}
				return fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, inputNotEmpty{})
			}
			extraRound = true
			before, _ = tg.Screen()
			cleared, err := pr.clearGxI()
			if err != nil {
				return err
			}
			gxCleared = cleared
			if pr.p.log != nil {
				pr.p.log.Info("input not empty, extra clear round", "session", pr.ss.session.SessionID, "cleared", cleared, "screen", before)
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

// notDelivered maps a stuck or unstarted send, and a slash read that could not
// tell, to agentrunner.ErrNotDelivered.
func notDelivered(err error) error {
	if errors.Is(err, agentrunner.ErrNotDelivered) {
		return err
	}
	if errors.Is(err, claudeui.ErrStuckSubmission) || errors.Is(err, claudeui.ErrStartTimeout) || errors.Is(err, deliver.ErrMaybeUnsent) {
		return fmt.Errorf("%w: %w", agentrunner.ErrNotDelivered, err)
	}
	return err
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

// slashCaps are the slash timings of a PTY prompt: the defaults, never longer
// than the runner's wait. Codex draws its unknown-command marker on screen only,
// so it sends once and watches the pane.
func (p *Pty) slashCaps(kind agentrunner.Kind) deliver.SlashCaps {
	caps := deliver.DefaultSlashCaps
	caps.Settle = min(caps.Settle, p.WaitN())
	caps.Gap = min(caps.Gap, p.WaitN())
	if kind == harness.Codex {
		caps.Settle = 0
		caps.Marker = codexui.UnknownCommandMarker
		caps.Watch = min(time.Second, p.WaitN())
	}
	return caps
}
