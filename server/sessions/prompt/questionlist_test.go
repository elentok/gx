package prompt_test

import (
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

func (f promptFixture) cardSources(t *testing.T) []string {
	t.Helper()
	chat, err := f.h.Client.Sessions.Chat(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range chat.Questions {
		out = append(out, c.Source)
	}
	return out
}

// A live blocker is the only card while it is open; the printed round returns
// once it clears.
func TestQuestionList_LiveBlockerHidesThePrintedRoundUntilItClears(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	writeRounds(t, f.transcript, 1)
	f.blockOn(t, agentfake.BashPermissionDialog("ls"))
	if got := f.cardSources(t); !slices.Equal(got, []string{wire.CardSourcePermission}) {
		t.Fatalf("blocked: sources = %v, want only permission", got)
	}
	report(t, f.conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if got := f.cardSources(t); !slices.Equal(got, []string{wire.CardSourceRound}) {
		t.Errorf("cleared: sources = %v, want the round", got)
	}
}
