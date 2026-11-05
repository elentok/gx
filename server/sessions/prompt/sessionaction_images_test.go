package prompt_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// failSend swallows every Enter a send makes, so its nudges give up and it
// raises a "Send failed" Actionable.
func (f promptFixture) failSend(t *testing.T, text string, ids ...string) wire.SessionActionable {
	t.Helper()
	for range 3 {
		if err := agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
			t.Fatal(err)
		}
	}
	if res := f.promptImages(t, text, ids...); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	return waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool {
		return strings.HasPrefix(s.Actionable.Reason, "Send failed")
	}).Actionable
}

func TestSessionAction_FailedSendKeepsItsImages(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	ids, _ := f.upload(t, 2)
	got := f.failSend(t, "look at these", ids...)
	if !slices.Equal(got.Images, ids) || !slices.Equal(got.Action, []string{"look at these"}) {
		t.Errorf("actionable = %+v, want images %v and the one text prompt", got, ids)
	}
}

func TestSessionAction_ReplayPastesEachImageThenTheText(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	ids, paths := f.upload(t, 2)
	f.failSend(t, "look at these", ids...)
	res, err := f.h.Client.Sessions.RunAction(f.ctx, f.id)
	if err != nil || res.Refused {
		t.Fatalf("run-action = %+v, %v", res, err)
	}
	waitForSubmitted(t, f.conv, wantEntry("look at these", paths...))
}
