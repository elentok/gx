package promptflows_test

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// runAction marks the chat actionable with steps and runs them.
func (c tuiChat) runAction(t *testing.T, steps ...string) {
	t.Helper()
	if res, err := c.h.Client.Sessions.MarkActionable(c.ctx, wire.MarkActionableRequest{ID: c.id, Reason: "ready", Action: steps}); err != nil || res.Refused {
		t.Fatalf("mark actionable = %+v, %v", res, err)
	}
	if res, err := c.h.Client.Sessions.RunAction(c.ctx, c.id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
}

// TestActionDeliver holds the Action step flows that type through Deliver.
func TestActionDeliver(t *testing.T) {
	t.Parallel()
	t.Run("UndecidedSlashStepRaisesTheCardAndDropsTheRest", func(t *testing.T) {
		t.Parallel()
		// No input box on screen: the settle read cannot tell whether the command ran.
		c := startTUIChat(t, agentfake.TUIPlainScreenEnv+"=1")
		c.runAction(t, "/first-command", "second-step")
		got := servertest.WaitForSession(c.ctx, t, c.h, c.evs, c.id, func(s wire.SessionInfo) bool { return s.Actionable.GxRaised }).Actionable
		if got.Kind != wire.DeliveryMaybeSent || !strings.HasSuffix(got.Reason, "1 later step not sent.") || len(got.Action) != 0 || got.CopyText != "/first-command" {
			t.Errorf("card = %+v, want the May not have been sent card with no Action", got)
		}
		time.Sleep(2 * firstPromptGrace) // negative wait: a later step would be typed within it
		if keys := c.keys(); !strings.Contains(keys, "/first-command") || strings.Contains(keys, "second-step") {
			t.Errorf("keys = %q, want the first step only", keys)
		}
	})
	t.Run("SwallowedEnterOnAPlainStepIsNudged", func(t *testing.T) {
		t.Parallel()
		c := startTUIChat(t)
		if err := agentfake.SendTUIControl(c.home, c.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
			t.Fatal(err)
		}
		c.runAction(t, "fix the bug")
		c.waitKeys(t, "the nudge Enter", func(k string) bool { return strings.Count(k, "\r") >= 2 })
		time.Sleep(2 * firstPromptGrace) // negative wait: a start timeout card would come within it
		if got := servertest.ReadMeta(t, c.h, c.id)["actionable"]; got != nil {
			t.Errorf("actionable = %v, want none: the turn started", got)
		}
		if n := strings.Count(c.keys(), "fix the bug"); n != 1 {
			t.Errorf("typed %d times, want once: %q", n, c.keys())
		}
	})
}
