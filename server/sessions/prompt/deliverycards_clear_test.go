package prompt_test

import (
	"strconv"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestDeliveryCardsClear(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{wire.DeliveryMaybeSent, wire.DeliverySendFailed, wire.DeliveryUnknownCommand} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := startPromptSession(t)
			f.seedActionable(t, wire.SessionActionable{Kind: kind, Reason: "card", CopyText: "x", GxRaised: true})
			f.prompt(t, "hello")
			waitForSubmitted(t, f.conv, strconv.Quote("hello")+"\n")
			report(t, f.conv, agentfake.Control{State: "working"})
			waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == "" })
		})
	}
}
