package promptflows_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// TestDeliveryCards holds one row per exit of a chat send: each raises its kind.
func TestDeliveryCards(t *testing.T) {
	t.Parallel()
	t.Run("PlainGiveUpIsSendFailedWithTheTextToResend", func(t *testing.T) {
		t.Parallel()
		f := startPromptSession(t)
		got := f.failSend(t, "do the thing")
		if !slices.Equal(got.Action, []string{"do the thing"}) || got.CopyText != "do the thing" || strings.HasPrefix(got.Reason, "Send failed") {
			t.Errorf("card = %+v, want the text in Action and CopyText and a reason without the old prefix", got)
		}
	})
	t.Run("PromptThatNeverReachesTheInputIsSendFailedWithResend", func(t *testing.T) {
		t.Parallel()
		c := startTUIChatPrompt(t, "fix it", agentfake.TUIDropTypedEnv+"=1")
		got := c.card(t)
		if got.Kind != wire.DeliverySendFailed || got.Reason != "prompt never reached the input" || !slices.Equal(got.Action, []string{"fix it"}) {
			t.Errorf("card = %+v, want the send-failed kind, reason %q and the prompt to resend", got, "prompt never reached the input")
		}
	})
	t.Run("UnknownCommandIsItsOwnKind", func(t *testing.T) {
		t.Parallel()
		c := startTUIChat(t, agentfake.TUIUnknownSkillEnv+"=1")
		c.send(t, "/my-skill")
		got := servertest.WaitForSession(c.ctx, t, c.h, c.evs, c.id, func(s wire.SessionInfo) bool { return s.Actionable.IsDelivery() }).Actionable
		if got.Kind != wire.DeliveryUnknownCommand || !strings.Contains(got.Reason, "Unknown command: /my-skill") || got.CopyText != "/my-skill" || !slices.Equal(got.Action, []string{"/my-skill"}) {
			t.Errorf("card = %+v, want the unknown-command kind with the text", got)
		}
	})
	t.Run("ResendOfUnknownCommandTypesTheCopyTextAndRaisesTheSameCard", func(t *testing.T) {
		t.Parallel()
		c := startTUIChat(t, agentfake.TUIUnknownSkillEnv+"=1")
		c.send(t, "/my-skill")
		first := c.card(t)
		if res, err := c.h.Client.Sessions.RunAction(c.ctx, c.id); err != nil || res.Refused {
			t.Fatalf("run action = %+v, %v", res, err)
		}
		c.waitKeys(t, "the second typing", func(k string) bool { return strings.Count(k, "/my-skill") >= 2 })
		got := servertest.WaitForSession(c.ctx, t, c.h, c.evs, c.id, func(s wire.SessionInfo) bool {
			return s.Actionable.IsDelivery() && s.Actionable.Reason != ""
		}).Actionable
		if got.Kind != first.Kind || got.CopyText != "/my-skill" || !strings.Contains(got.Reason, "Unknown command: /my-skill") || strings.HasPrefix(got.Reason, "Action failed") {
			t.Errorf("card = %+v, want the same unknown-command card as %+v", got, first)
		}
	})
	t.Run("UndecidedSlashSendIsMaybeSentWithNoResend", func(t *testing.T) {
		t.Parallel()
		c := startTUIChat(t, agentfake.TUIPlainScreenEnv+"=1")
		c.send(t, "/co")
		got := servertest.WaitForSession(c.ctx, t, c.h, c.evs, c.id, func(s wire.SessionInfo) bool { return s.Actionable.IsDelivery() }).Actionable
		if got.Kind != wire.DeliveryMaybeSent || got.CopyText != "/co" || len(got.Action) != 0 || !strings.Contains(got.Reason, "See the screen") {
			t.Errorf("card = %+v, want the maybe-sent kind with the text and no Action", got)
		}
	})
}
