package server

import (
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
