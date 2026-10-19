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
	state := harness.InputUndecided
	if c.Supports(nativerunner.OpScreenStyled) {
		if rows, err := c.StyledScreen(); err == nil {
			state = sessionTyping.InputIsEmpty(rows)
		}
	}
	holds := state == harness.InputNotEmpty
	s.log.Debug("draft check", "session", filepath.Base(dir), "holds_draft", holds)
	return holds
}

// pressEnterIfStillTyped is the rule for a slash prompt that ended without
// the session reacting: when the harness empty check says the prompt is still
// in the input, press Enter once. An empty input, or a check that can't
// decide, presses nothing.
func pressEnterIfStillTyped(dir string) (pressed bool, err error) {
	c, err := nativerunner.DialHolder(dir)
	if err != nil || !c.Supports(nativerunner.OpScreenStyled) {
		return false, err
	}
	rows, err := c.StyledScreen()
	if err != nil || sessionTyping.InputIsEmpty(rows) != harness.InputNotEmpty {
		return false, err
	}
	return true, c.SendKeys("\r")
}
