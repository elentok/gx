package prompt

import (
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/wire"
)

// sessionTyping is the only harness the server types into. It serves the parts
// that read no env; ReadyToType reads claude's config, so it goes through
// projects.Registry.
var sessionTyping = harness.ClaudeTyping()

// ErrDraft means a person's draft would not clear, so gx did not type.
var ErrDraft = deliver.ErrDraft

// DraftReason is the reason of the Actionable gx raises when a draft would
// not clear.
const DraftReason = "input would not clear"

// draftHooks is what the draft clear tells the person: a notice when it typed
// on an untrusted read, and none left once a read was trusted.
func (p *Prompt) draftHooks(dir string) *deliver.DraftHooks {
	return &deliver.DraftHooks{
		Untrusted: func() { p.RaiseNotice(dir, wire.SessionNoticeUntrustedRead) },
		Decided:   func() { p.ClearNotices(dir) },
	}
}

// ClearDraft empties a person's draft before gx types (the Action steps; chat
// sends clear it inside deliver.Deliver).
func (p *Prompt) ClearDraft(dir string, c *holder.HolderClient) error {
	log := p.log.With("session", filepath.Base(dir))
	return deliver.ClearDraft(c, sessionTyping, log, *p.draftHooks(dir))
}

// InputState runs the harness empty check when the holder serves styled rows.
// A holder that doesn't, or a read that fails, is InputUndecided; the error is
// for callers that report it.
//
// A read taken after a PTY resize, before the child's next output, is
// untrusted: the screen is still what the child drew for the old size.
func InputState(c *holder.HolderClient) (harness.InputState, error) {
	state, _, err := inputRead(c)
	return state, err
}

// inputRead is InputState with the read, whose Resized flag tells an untrusted
// read from one that simply can't decide.
func inputRead(c *holder.HolderClient) (harness.InputState, holder.ReadState, error) {
	return deliver.InputRead(c, sessionTyping)
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
