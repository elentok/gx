package prompt

import (
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/server/wire"
)

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
