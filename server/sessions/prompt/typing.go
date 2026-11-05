package prompt

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
)

// sessionTyping is the only harness the server types into. It serves the parts
// that read no env; ReadyToType reads claude's config, so it goes through
// projects.Registry.
var sessionTyping = harness.ClaudeTyping()

// ErrDraft means a person's draft would not clear, so gx did not type.
var ErrDraft = errors.New("input would not clear")

// DraftReason is the reason of the Actionable gx raises when a draft would
// not clear.
const DraftReason = "input would not clear"

// Clearing a draft: each round presses the clear key draftClearPresses times
// (Ctrl+U on an empty input does nothing), then rereads the input until it
// reads empty or draftSettleWait passes. The 32 presses clear a draft of up to
// 16 lines.
const (
	draftClearRounds  = 4
	draftClearPresses = 8
	draftSettleWait   = time.Second
	draftSettlePoll   = 25 * time.Millisecond
)

// ClearDraft empties a person's draft before gx types. The placeholder is not
// a draft: the harness check tells them apart by its dim style. A check that
// can't decide types as before.
func (p *Prompt) ClearDraft(dir string, c *holder.HolderClient) error {
	log := p.log.With("session", filepath.Base(dir))
	if state, _ := InputState(c); state != harness.InputNotEmpty {
		log.Debug("draft check", "holds_draft", false)
		return nil
	}
	for range draftClearRounds {
		for range draftClearPresses {
			if err := c.SendKeys(sessionTyping.ClearKey()); err != nil {
				return err
			}
		}
		// The agent redraws a moment after the keys land.
		settle := time.Now().Add(draftSettleWait)
		for {
			state, err := InputState(c)
			if err != nil {
				return err
			}
			if state != harness.InputNotEmpty {
				log.Info("cleared a draft before typing")
				return nil
			}
			if time.Now().After(settle) {
				break
			}
			time.Sleep(draftSettlePoll)
		}
	}
	rows, _ := c.StyledScreen()
	log.Info("draft would not clear", "screen", strings.Join(rows, "\n"))
	return ErrDraft
}

// InputState runs the harness empty check when the holder serves styled rows.
// A holder that doesn't, or a read that fails, is InputUndecided; the error is
// for callers that report it.
func InputState(c *holder.HolderClient) (harness.InputState, error) {
	if !c.Supports(holder.OpScreenStyled) {
		return harness.InputUndecided, nil
	}
	rows, err := c.StyledScreen()
	if err != nil {
		return harness.InputUndecided, err
	}
	return sessionTyping.InputIsEmpty(rows), nil
}

// settleEnters caps the Enters PressEnterIfStillTyped presses, enterGap apart.
// Claude holds a paste it stripped invisible characters from for review and
// swallows an Enter or two before it sends.
var (
	settleEnters = 3
	enterGap     = time.Second
)

// PressEnterIfStillTyped is the rule for a slash prompt that ended without
// the session reacting: while the harness empty check says the prompt is still
// in the input, press Enter, up to settleEnters times. An empty input, or a
// check that can't decide, presses nothing.
func (p *Prompt) PressEnterIfStillTyped(dir string, holder *holder.HolderClient) (pressed bool, err error) {
	log := p.log.With("session", filepath.Base(dir))
	for i := range settleEnters {
		state, err := InputState(holder)
		if err != nil || state != harness.InputNotEmpty {
			log.Info("input check at settle", "attempt", i, "state", state, "err", err)
			return pressed, err
		}
		if err := holder.SendKeys("\r"); err != nil {
			return pressed, err
		}
		pressed = true
		log.Info("settle enter", "attempt", i)
		time.Sleep(enterGap)
	}
	return pressed, nil
}
