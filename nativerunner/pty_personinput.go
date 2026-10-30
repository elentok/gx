package nativerunner

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
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
	ss, err := p.live(s)
	if err != nil {
		return nil, nil, err
	}
	if !ss.holder.Supports(OpWatch) {
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
			if m.Type != StreamInput {
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
	p       *Pty
	ss      *ptySession
	turn    int
	person  <-chan struct{}
	typed   bool
	stopped bool
}

// cancelled returns errPersonInput once a person typed after gx did.
func (pr *ptyPrompter) cancelled() error {
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

// awaitEmptyInput waits up to wait for a person's draft to go, so gx never
// types over it. A check that can't decide types as before.
func (pr *ptyPrompter) awaitEmptyInput(wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		empty, known, err := pr.InputEmpty()
		if err != nil || empty || !known {
			return err
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrInputNotEmpty)
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
	if err := pr.ss.holder.TypeText(text); err != nil {
		return pr.p.died(pr.ss, err)
	}
	pr.typed = true
	return pr.SendEnter()
}

// Clear presses the harness clear key once.
func (pr *ptyPrompter) Clear() error {
	if err := pr.cancelled(); err != nil {
		return err
	}
	return pr.p.died(pr.ss, pr.ss.holder.SendKeys(pr.p.typing().ClearKey()))
}

// InputEmpty runs the harness empty check when the holder serves styled rows.
func (pr *ptyPrompter) InputEmpty() (bool, bool, error) {
	if !pr.ss.holder.Supports(OpScreenStyled) {
		return false, false, nil
	}
	rows, err := pr.ss.holder.StyledScreen()
	if err != nil {
		return false, false, pr.p.died(pr.ss, err)
	}
	switch pr.p.typing().InputIsEmpty(rows) {
	case harness.InputEmpty:
		return true, true, nil
	case harness.InputNotEmpty:
		return false, true, nil
	}
	return false, false, nil
}

// ReadyToType runs claude's ready-to-type step over the holder.
func (pr *ptyPrompter) ReadyToType() bool {
	return pr.p.typing().ReadyToType(HolderTypingIO(pr.ss.holder))
}

// HolderTypingIO reads styled rows when the holder serves them, plain rows
// otherwise; the ready-to-type step needs only the text.
func HolderTypingIO(h *HolderClient) harness.TypingIO {
	return harness.TypingIO{
		Rows: func() ([]string, error) {
			if h.Supports(OpScreenStyled) {
				return h.StyledScreen()
			}
			rows, _, err := h.Screen(0)
			return rows, err
		},
		SendKeys: h.SendKeys,
	}
}

// claudeTyping is the holder child's typing rules; it reads the env the
// parent gave that child.
var claudeTyping = harness.ClaudeTyping()

// typing is claude's typing rules reading its config from p.Env. A nil Env
// becomes an empty one, which harness would read as the process env.
func (p *Pty) typing() harness.Typing {
	env := p.Env
	if env == nil {
		env = []string{}
	}
	return harness.ClaudeTypingIn(env)
}

func (pr *ptyPrompter) SendEnter() error {
	if err := pr.cancelled(); err != nil {
		return err
	}
	return pr.p.died(pr.ss, pr.ss.holder.SendKeys("\r"))
}

func (pr *ptyPrompter) WaitStart(timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		if err := pr.cancelled(); err != nil {
			return false, err
		}
		_, turn, err := pr.ss.holder.Status()
		if err != nil {
			return false, pr.p.died(pr.ss, err)
		}
		if turn > pr.turn {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-pr.ss.exited:
			return false, pr.p.died(pr.ss, errors.New("holder exited"))
		case <-time.After(pr.p.pollInterval()):
		}
	}
}

func (pr *ptyPrompter) Screen() (string, error) {
	rows, _, err := pr.ss.holder.Screen(0)
	return strings.Join(rows, "\n"), pr.p.died(pr.ss, err)
}
