package designflows

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

// cardSources is the sources of the cards the session's chat lists.
func (r *running) cardSources(t *testing.T, id string) []string {
	t.Helper()
	var chat wire.SessionChatResponse
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", id, "--json")), &chat); err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, c := range chat.Questions {
		sources = append(sources, c.Source)
	}
	return sources
}

func TestBlockedScenario_EachKindOfQuestionListsItsCard(t *testing.T) {
	t.Parallel()
	r := sharedBlocked(t)
	list := r.seededSessions(t)
	if len(list) != commandSessionCount {
		t.Fatalf("%d sessions, want %d", len(list), commandSessionCount)
	}
	first := 22
	for i, want := range []string{
		wire.CardSourceClaudeDialog, wire.CardSourceClaudeDialog, wire.CardSourceCodex,
		wire.CardSourcePermission, wire.CardSourcePermission, wire.CardSourceRound,
	} {
		id := list[first+i].ID
		if !testutil.Poll(20*time.Second, 200*time.Millisecond, func() bool {
			return slices.Contains(r.cardSources(t, id), want)
		}) {
			t.Errorf("question session %d lists %v, want a %s card", i, r.cardSources(t, id), want)
		}
	}
}

func TestBlockedScenario_TheAskingQueueAgentListsATicketAskCard(t *testing.T) {
	t.Parallel()
	r := sharedBlocked(t)
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		for _, si := range r.sessionsOf(t) {
			if si.Queue && slices.Contains(r.cardSources(t, si.ID), wire.CardSourceTicketAsk) {
				return true
			}
		}
		return false
	}) {
		t.Fatal("no Queue agent lists a ticket-ask card")
	}
}
