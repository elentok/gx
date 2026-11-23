package promptflows_test

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

const firstSlash = "/gx-implement gx:epic/01"

// TestFirstPromptDeliver holds the first-prompt flows that type through Deliver.
func TestFirstPromptDeliver(t *testing.T) {
	t.Parallel()
	t.Run("SwallowedEnterOnASlashFirstPromptLandsWithOneRetypeFreeEnterAndNoCard", func(t *testing.T) {
		t.Parallel()
		c := startTUIChatPrompt(t, firstSlash, agentfake.TUISwallowEnterEnv+"=1")
		c.waitKeys(t, "the extra Enter", func(k string) bool { return strings.Count(k, "\r") >= 2 })
		servertest.WaitForSession(c.ctx, t, c.h, c.evs, c.id, func(s wire.SessionInfo) bool {
			return s.Status.State == wire.SessionWorking || s.Status.State == wire.SessionIdle
		})
		time.Sleep(2 * firstPromptGrace) // negative wait: a retype or a card would come within it
		if n := strings.Count(c.keys(), firstSlash); n != 1 {
			t.Errorf("typed %d times, want once: %q", n, c.keys())
		}
		if got := servertest.ReadMeta(t, c.h, c.id)["actionable"]; got != nil {
			t.Errorf("actionable = %v, want none", got)
		}
	})
	t.Run("UndecidedSlashFirstPromptRaisesNoMaybeSentCardAndTheProofDecides", func(t *testing.T) {
		t.Parallel()
		// No input box on screen: the settle read cannot tell whether the command ran.
		c := startTUIChatPrompt(t, firstSlash, agentfake.TUIUndecidedEnv+"=1")
		got := servertest.WaitForSession(c.ctx, t, c.h, c.evs, c.id, func(s wire.SessionInfo) bool { return s.Actionable.GxRaised }).Actionable
		if strings.HasPrefix(got.Reason, "May not have been sent") {
			t.Errorf("reason = %q, want the launch proof's verdict, not the undecided card", got.Reason)
		}
		if n := strings.Count(c.keys(), firstSlash); n != 1 {
			t.Errorf("typed %d times, want once: %q", n, c.keys())
		}
	})
}
