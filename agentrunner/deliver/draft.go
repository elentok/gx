package deliver

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
)

// ErrDraft means a person's draft would not clear, so nothing was typed.
var ErrDraft = errors.New("input would not clear")

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

// DraftHooks turn the clear-draft step on and tell the caller what it saw.
type DraftHooks struct {
	// Untrusted runs when the read was resized or could not decide: typing goes
	// on without knowing whether a draft is there.
	Untrusted func()
	// Decided runs when the read was trusted and the input is empty or cleared.
	Decided func()
}

// DraftClearer is the part of an Input that can clear a person's draft.
type DraftClearer interface {
	ClearDraft(DraftHooks) error
}

// ClearDraft empties a person's draft before gx types. The placeholder is not a
// draft: the harness check tells them apart by its dim style. A check that
// can't decide types as before. The caller holds the input.
func ClearDraft(c *holder.HolderClient, typing harness.Typing, log *slog.Logger, hooks DraftHooks) error {
	state, read, _ := InputRead(c, typing)
	if read.Resized || state == harness.InputUndecided {
		log.Info("typing on an untrusted screen read", "resized", read.Resized, "pty_size", read.PTY, "attach_sizes", read.Attaches)
		if hooks.Untrusted != nil {
			hooks.Untrusted()
		}
		return nil
	}
	decided := func() {
		if hooks.Decided != nil {
			hooks.Decided()
		}
	}
	if state != harness.InputNotEmpty {
		log.Debug("draft check", "holds_draft", false)
		decided()
		return nil
	}
	for range draftClearRounds {
		for range draftClearPresses {
			if err := c.SendKeys(typing.ClearKey()); err != nil {
				return holderErr(err)
			}
		}
		// The agent redraws a moment after the keys land.
		settle := time.Now().Add(draftSettleWait)
		for {
			state, _, err := InputRead(c, typing)
			if err != nil {
				return err
			}
			if state != harness.InputNotEmpty {
				log.Info("cleared a draft before typing")
				decided()
				return nil
			}
			if time.Now().After(settle) {
				break
			}
			time.Sleep(draftSettlePoll)
		}
	}
	rows, read, _ := c.StyledScreenRead()
	log.Info("draft would not clear", "screen", strings.Join(rows, "\n"), "pty_size", read.PTY, "attach_sizes", read.Attaches)
	return ErrDraft
}

// ClearDraft clears a draft under the typing lock and one batch.
func (t *Target) ClearDraft(hooks DraftHooks) error {
	if err := t.cancelled(); err != nil {
		return err
	}
	release, err := t.acquire()
	if err != nil {
		return err
	}
	defer release()
	return ClearDraft(t.C, t.Typing, logOf(t), hooks)
}
