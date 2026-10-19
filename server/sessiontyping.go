package server

import (
	"errors"
	"path/filepath"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// sessionTyping is the only harness the server types into.
var sessionTyping = func() harness.Typing {
	t, err := harness.LookupTyping(harness.Claude)
	if err != nil {
		panic(err)
	}
	return t
}()

// draftReason is the reason of the Actionable gx raises instead of typing
// into a draft.
const draftReason = "input holds a draft"

// errDraft stops a prompt gx would have typed over a person's draft.
var errDraft = errors.New(draftReason)

// inputHoldsDraft runs the harness empty check before gx types. Only a
// definite "not empty" refuses; a check that can't decide types as before.
func (s *Server) inputHoldsDraft(dir string, c *nativerunner.HolderClient) bool {
	state, _ := inputState(c)
	holds := state == harness.InputNotEmpty
	s.log.Debug("draft check", "session", filepath.Base(dir), "holds_draft", holds)
	return holds
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

// pressEnterIfStillTyped is the rule for a slash prompt that ended without
// the session reacting: when the harness empty check says the prompt is still
// in the input, press Enter once. An empty input, or a check that can't
// decide, presses nothing.
func (r *actionRun) pressEnterIfStillTyped() (pressed bool, err error) {
	state, err := inputState(r.holder)
	if err != nil || state != harness.InputNotEmpty {
		return false, err
	}
	return true, r.holder.SendKeys("\r")
}
