package server

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// sessionTyping is the only harness the server types into. It serves the parts
// that read no env; ReadyToType reads claude's config, so it goes through
// Server.typing.
var sessionTyping = harness.ClaudeTyping()

// errDraft means a person's draft would not clear, so gx did not type.
var errDraft = errors.New("input would not clear")

// draftReason is the reason of the Actionable gx raises when a draft would
// not clear.
const draftReason = "input would not clear"

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

// clearDraft empties a person's draft before gx types. The placeholder is not
// a draft: the harness check tells them apart by its dim style. A check that
// can't decide types as before.
func (s *Server) clearDraft(dir string, c *nativerunner.HolderClient) error {
	log := s.log.With("session", filepath.Base(dir))
	if state, _ := inputState(c); state != harness.InputNotEmpty {
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
			state, err := inputState(c)
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
	return errDraft
}

// inputState runs the harness empty check when the holder serves styled rows.
// A holder that doesn't, or a read that fails, is InputUndecided; the error is
// for callers that report it.
func inputState(c *nativerunner.HolderClient) (harness.InputState, error) {
	if !c.Supports(nativerunner.OpScreenStyled) {
		return harness.InputUndecided, nil
	}
	rows, err := c.StyledScreen()
	if err != nil {
		return harness.InputUndecided, err
	}
	return sessionTyping.InputIsEmpty(rows), nil
}

// settleEnters caps the Enters pressEnterIfStillTyped presses, enterGap apart.
// Claude holds a paste it stripped invisible characters from for review and
// swallows an Enter or two before it sends.
var (
	settleEnters = 3
	enterGap     = time.Second
)

// pressEnterIfStillTyped is the rule for a slash prompt that ended without
// the session reacting: while the harness empty check says the prompt is still
// in the input, press Enter, up to settleEnters times. An empty input, or a
// check that can't decide, presses nothing.
func (r *actionRun) pressEnterIfStillTyped() (pressed bool, err error) {
	log := r.s.log.With("session", filepath.Base(r.dir))
	for i := range settleEnters {
		state, err := inputState(r.holder)
		if err != nil || state != harness.InputNotEmpty {
			log.Info("input check at settle", "attempt", i, "state", state, "err", err)
			return pressed, err
		}
		if err := r.holder.SendKeys("\r"); err != nil {
			return pressed, err
		}
		pressed = true
		log.Info("settle enter", "attempt", i)
		time.Sleep(enterGap)
	}
	return pressed, nil
}
