package askflows_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func (f askFixture) answer(t *testing.T, req wire.SessionAnswerRequest) wire.SessionResult {
	t.Helper()
	req.ID, req.Source = f.id, wire.CardSourceClaudeDialog
	if req.CardID == "" {
		req.CardID = "toolu_ask"
	}
	res, err := f.h.Client.Sessions.Answer(f.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// hookResult waits for the hook to exit and returns what it printed.
func hookResult(t *testing.T, path string) string {
	t.Helper()
	var out string
	servertest.WaitFor(t, 10*time.Second, "the hook to exit", func() bool {
		b, err := os.ReadFile(path)
		out = string(b)
		return err == nil && len(b) > 0
	})
	return out
}

type hookOutput struct {
	HookSpecificOutput struct {
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
		UpdatedInput             struct {
			Questions   []map[string]any  `json:"questions"`
			Answers     map[string]string `json:"answers"`
			Annotations map[string]any    `json:"annotations"`
		} `json:"updatedInput"`
	} `json:"hookSpecificOutput"`
}

func TestAskAnswer_AnswersKeyedByQuestionTextWithNotes(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	out := f.ask(t)
	f.waitBlocked(t)
	res := f.answer(t, wire.SessionAnswerRequest{Answers: []wire.SessionAnswer{
		{Choice: "2", Note: "blue please"},
		{Choices: []string{"1", "3", "other"}, Text: "XL"},
		{Choice: "other", Text: "maybe"},
	}})
	if res.Refused {
		t.Fatalf("answer refused: %+v", res)
	}
	f.waitIdle(t)
	var got hookOutput
	if err := json.Unmarshal([]byte(hookResult(t, out)), &got); err != nil {
		t.Fatal(err)
	}
	h := got.HookSpecificOutput
	if h.PermissionDecision != "allow" {
		t.Errorf("decision = %q, want allow", h.PermissionDecision)
	}
	want := map[string]string{"Which color?": "blue", "Which sizes?": "S, L, XL", "Ship it?": "maybe"}
	if !reflect.DeepEqual(h.UpdatedInput.Answers, want) {
		t.Errorf("answers = %v, want %v", h.UpdatedInput.Answers, want)
	}
	if len(h.UpdatedInput.Questions) != 3 || h.UpdatedInput.Questions[1]["question"] != "Which sizes?" {
		t.Errorf("questions = %v, want the original three", h.UpdatedInput.Questions)
	}
	if !reflect.DeepEqual(h.UpdatedInput.Annotations, map[string]any{"Which color?": map[string]any{"notes": "blue please"}}) {
		t.Errorf("annotations = %v", h.UpdatedInput.Annotations)
	}
}

func TestAskAnswer_DismissDeniesWithTheExactReason(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	out := f.ask(t)
	f.waitBlocked(t)
	if res := f.answer(t, wire.SessionAnswerRequest{}); res.Refused {
		t.Fatalf("dismiss refused: %+v", res)
	}
	f.waitIdle(t)
	var got hookOutput
	if err := json.Unmarshal([]byte(hookResult(t, out)), &got); err != nil {
		t.Fatal(err)
	}
	const reason = "The user doesn't want to proceed with this question. Do not ask again. STOP and wait for the user to tell you how to proceed."
	if h := got.HookSpecificOutput; h.PermissionDecision != "deny" || h.PermissionDecisionReason != reason {
		t.Errorf("hook = %+v, want deny with the dismiss reason", h)
	}
}

func TestAskAnswer_ReleasePrintsNothingAndClearsTheCard(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	out := f.ask(t)
	f.waitBlocked(t)
	if res := f.answer(t, wire.SessionAnswerRequest{Release: true}); res.Refused {
		t.Fatalf("release refused: %+v", res)
	}
	f.waitIdle(t)
	if b, _ := os.ReadFile(out); len(b) != 0 {
		t.Errorf("hook printed %q, want nothing", b)
	}
	if cards := f.cards(t); len(cards) != 0 {
		t.Errorf("cards after release = %+v, want none", cards)
	}
	// The same dialog asked again is released at once and shows no card.
	f.ask(t)
	f.waitIdle(t)
	if cards := f.cards(t); len(cards) != 0 {
		t.Errorf("cards for the released id = %+v, want none", cards)
	}
}

func TestAskAnswer_StaleOrIncompleteSendsNothing(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	out := f.ask(t)
	f.waitBlocked(t)
	if res := f.answer(t, wire.SessionAnswerRequest{CardID: "toolu_old", Answers: []wire.SessionAnswer{{Choice: "1"}}}); !res.Refused || res.Reason != wire.ReasonStale {
		t.Errorf("stale id: %+v, want %s", res, wire.ReasonStale)
	}
	partial := []wire.SessionAnswer{{Choice: "1"}, {}, {Choice: "2"}}
	if res := f.answer(t, wire.SessionAnswerRequest{Answers: partial}); !res.Refused || res.Reason != wire.ReasonSessionBadRequest {
		t.Errorf("partial answer: %+v, want %s", res, wire.ReasonSessionBadRequest)
	}
	if b, _ := os.ReadFile(out); len(b) != 0 {
		t.Errorf("hook printed %q, want nothing", b)
	}
	if cards := f.cards(t); len(cards) != 1 {
		t.Errorf("cards = %+v, want the card kept", cards)
	}
}
