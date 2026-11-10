package prompt_test

import (
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// permissionCard is the chat's one permission card of a blocked session.
func (f promptFixture) permissionCard(t *testing.T) wire.QuestionCard {
	t.Helper()
	chat, err := f.h.Client.Sessions.Chat(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.Questions) != 1 || chat.Questions[0].Source != wire.CardSourcePermission {
		t.Fatalf("questions = %+v, want one permission card", chat.Questions)
	}
	return chat.Questions[0]
}

func TestPermissionCard_ListsEveryOptionWithItsExactLabel(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.WrappedPermissionDialog("ls"))
	card := f.permissionCard(t)
	if len(card.Questions) != 1 {
		t.Fatalf("questions = %+v, want one", card.Questions)
	}
	q := card.Questions[0]
	var labels []string
	for _, o := range q.Options {
		labels = append(labels, o.Label)
	}
	want := []string{"Yes", "Yes, and don't ask again for this command in /some/long/project/path", "No"}
	if q.Kind != "options" || !slices.Equal(labels, want) || q.Recommended != "" {
		t.Errorf("question = %+v, want options %q and no recommended mark", q, want)
	}
}

func TestPermissionCard_AnswerSendsTheDigitAndNothingElse(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.WrappedPermissionDialog("ls"))
	card := f.permissionCard(t)
	res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: card.ID, Answers: []wire.SessionAnswer{{Choice: "2"}}})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	waitForSubmitted(t, f.conv, `"2"`+"\n")
}

func TestPermissionCard_DismissSendsWhatDenySends(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.BashPermissionDialog("ls"))
	card := f.permissionCard(t)
	if res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: card.ID}); res.Refused {
		t.Fatalf("dismiss = %+v", res)
	}
	waitForSubmitted(t, f.conv, escLogged)
}

func TestPermissionCard_StaleIdAndUnknownChoiceSendNothing(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.BashPermissionDialog("ls"))
	card := f.permissionCard(t)
	if res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: "old", Answers: []wire.SessionAnswer{{Choice: "1"}}}); !res.Refused || res.Reason != wire.ReasonStale {
		t.Errorf("stale id: %+v, want %s", res, wire.ReasonStale)
	}
	if res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: card.ID, Answers: []wire.SessionAnswer{{Choice: "9"}}}); !res.Refused || res.Reason != wire.ReasonSessionBadRequest {
		t.Errorf("unknown choice: %+v, want %s", res, wire.ReasonSessionBadRequest)
	}
	f.nothingSent(t)
}

func TestPermissionCard_UnreadableScreenListsNoCardAndKeepsTheBanner(t *testing.T) {
	t.Parallel()
	for name, rows := range map[string][]string{
		"unreadable text": {"something no one can read"},
		"login screen": {
			"Select login method:",
			"❯ 1. Claude account with subscription",
			"  2. Anthropic Console account",
			"Esc to cancel",
		},
		"MCP elicitation": {
			"────────────────────",
			"Server fixture-mcp requests information",
			"  Enter the deployment target",
			"❯ 1. Accept",
			"  2. Decline",
			"Esc to cancel",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startPromptSessionWithGrace(t, 10*time.Second)
			f.blockOn(t, rows)
			chat, err := f.h.Client.Sessions.Chat(f.ctx, f.id)
			if err != nil {
				t.Fatal(err)
			}
			if len(chat.Questions) != 0 {
				t.Errorf("questions = %+v, want none", chat.Questions)
			}
			if chat.Failure == nil || chat.Failure.Kind != wire.FailureKindBlocked {
				t.Errorf("failure = %+v, want the blocked banner", chat.Failure)
			}
		})
	}
}
