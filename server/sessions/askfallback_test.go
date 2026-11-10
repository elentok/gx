package sessions_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// block makes the fake wait on waitingFor showing rows, as claude does for a
// dialog the hook did not catch.
func (f askFixture) block(t *testing.T, waitingFor string, rows []string) {
	t.Helper()
	c := agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: waitingFor, Dialog: rows}
	if err := agentfake.SendTUIControl(f.home, f.conv, c); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.esc() })
}

// waitCard polls the chat until a card lists.
func (f askFixture) waitCard(t *testing.T) wire.QuestionCard {
	t.Helper()
	var card wire.QuestionCard
	testutil.WaitFor(t, 10*time.Second, "a card to list", func() bool {
		cards := f.cards(t)
		if len(cards) > 0 {
			card = cards[0]
		}
		return len(cards) > 0
	})
	return card
}

// noCardFor polls the chat for longer than the fallback delay: a negative has
// no event to wait for.
func (f askFixture) noCardFor(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cards := f.cards(t); len(cards) != 0 {
			t.Fatalf("cards = %+v, want none", cards)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestAskFallback_InputNeededListsACardWithNoNote(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	f.block(t, "input needed", agentfake.AskDialogRows("Which color?", "red", "blue"))
	card := f.waitCard(t)
	if card.Source != wire.CardSourceClaudeDialog || !card.Fallback || len(card.Questions) != 1 {
		t.Fatalf("card = %+v", card)
	}
	q := card.Questions[0]
	if q.Title != "Which color?" || q.Note != "" || len(q.Options) != 3 || q.Options[0].Label != "red" || !q.Options[2].Text {
		t.Errorf("question = %+v", q)
	}
}

func TestAskFallback_PickerAndPermissionPromptListNoCard(t *testing.T) {
	t.Parallel()
	for name, waitingFor := range map[string]string{"picker": "dialog open", "permission": "permission prompt"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startAskFixture(t, nil)
			f.block(t, waitingFor, agentfake.AskDialogRows("Which color?", "red", "blue"))
			f.noCardFor(t)
		})
	}
}

func TestAskFallback_UnreadableLayoutListsNoCard(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	rows := agentfake.AskDialogRows("Which color?", "red", "blue")
	for i, r := range rows {
		if len(r) > 0 && r[0:len("❯")] == "❯" {
			rows[i] = "  " + r[len("❯"):]
		}
	}
	f.block(t, "input needed", rows)
	f.noCardFor(t)
}

func TestAskFallback_HookTimeoutClearsTheHookCardAndListsTheFallback(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	call := &agentfake.AskCall{
		ToolUseID: "toolu_ask", TimeoutMs: 1500,
		Questions: []agentfake.AskQuestion{{Header: "Color", Question: "Which color?", Options: []agentfake.AskOption{{Label: "red"}, {Label: "blue"}}}},
		Dialog:    agentfake.AskDialogRows("Which color?", "red", "blue"),
	}
	if err := agentfake.SendTUIControl(f.home, f.conv, agentfake.Control{Action: agentfake.ActionAskQuestion, Ask: call}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.esc() })
	if card := f.waitCard(t); card.ID != "toolu_ask" {
		t.Fatalf("first card = %q, want the hook card", card.ID)
	}
	testutil.WaitFor(t, 10*time.Second, "the fallback card to replace the hook card", func() bool {
		cards := f.cards(t)
		return len(cards) == 1 && cards[0].ID != "toolu_ask"
	})
}

func TestAskFallback_ReleasedDialogListsNoFallbackWhileOpen(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	call := &agentfake.AskCall{
		ToolUseID: "toolu_ask",
		Questions: []agentfake.AskQuestion{{Header: "Color", Question: "Which color?", Options: []agentfake.AskOption{{Label: "red"}, {Label: "blue"}}}},
		Dialog:    agentfake.AskDialogRows("Which color?", "red", "blue"),
	}
	if err := agentfake.SendTUIControl(f.home, f.conv, agentfake.Control{Action: agentfake.ActionAskQuestion, Ask: call}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.esc() })
	f.waitCard(t)
	if res := f.answer(t, wire.SessionAnswerRequest{Release: true}); res.Refused {
		t.Fatalf("release refused: %+v", res)
	}
	f.waitBlocked(t)
	f.noCardFor(t)
}
