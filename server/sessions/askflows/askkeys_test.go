package askflows_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

func askOpts(labels ...string) []agentfake.AskOption {
	var out []agentfake.AskOption
	for _, l := range labels {
		out = append(out, agentfake.AskOption{Label: l, Description: "about " + l})
	}
	return out
}

var (
	colorQ = agentfake.AskQuestion{Header: "Color", Question: "Which color?", Options: askOpts("red", "blue")}
	sizesQ = agentfake.AskQuestion{Header: "Sizes", Question: "Which sizes?", MultiSelect: true, Options: askOpts("S", "M", "L")}
	nameQ  = agentfake.AskQuestion{Header: "Name", Question: "Which name?", Options: askOpts("Ann", "Bob")}
)

// keyDialog is an interactive dialog the fake shows with no hook; the keys it
// reads and the answers it ends with land in files.
type keyDialog struct {
	keys, answers string
}

func (f askFixture) openDialog(t *testing.T, mutateAfter int, qs ...agentfake.AskQuestion) keyDialog {
	t.Helper()
	dir := t.TempDir()
	d := keyDialog{keys: filepath.Join(dir, "keys"), answers: filepath.Join(dir, "answers")}
	screen := &agentfake.AskScreen{Questions: qs, KeysFile: d.keys, AnswersFile: d.answers, MutateAfterKeys: mutateAfter}
	if err := agentfake.SendTUIControl(f.home, f.conv, agentfake.Control{Action: agentfake.ActionAskScreen, AskScreen: screen}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.esc() })
	return d
}

// recorded is the keys the fake read, one per line.
func (d keyDialog) recorded() []string {
	b, err := os.ReadFile(d.keys)
	if err != nil || len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// ended waits for the dialog to end and returns what it ended with.
func (d keyDialog) ended(t *testing.T) string {
	t.Helper()
	var out string
	servertest.WaitFor(t, 10*time.Second, "the dialog to end", func() bool {
		b, err := os.ReadFile(d.answers)
		out = string(b)
		return err == nil && len(b) > 0
	})
	return out
}

func (d keyDialog) endedWith(t *testing.T, want map[string]string) {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal([]byte(d.ended(t)), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("answers = %v (%v), want %v", got, err, want)
	}
}

func (d keyDialog) wantKeys(t *testing.T, want ...string) {
	t.Helper()
	if got := d.recorded(); !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func chars(s string) []string { return strings.Split(s, "") }

// waitCardTitle polls until the card is for the question title.
func (f askFixture) waitCardTitle(t *testing.T, title string) wire.QuestionCard {
	t.Helper()
	var card wire.QuestionCard
	servertest.WaitFor(t, 10*time.Second, "the card of "+title, func() bool {
		for _, c := range f.cards(t) {
			if len(c.Questions) == 1 && c.Questions[0].Title == title {
				card = c
				return true
			}
		}
		return false
	})
	return card
}

func (f askFixture) answerCard(t *testing.T, card wire.QuestionCard, a wire.SessionAnswer) wire.SessionResult {
	t.Helper()
	return f.answer(t, wire.SessionAnswerRequest{CardID: card.ID, Answers: []wire.SessionAnswer{a}})
}

func TestAskKeys_SingleSelectMovesDownThenEnter(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	d := f.openDialog(t, 0, colorQ)
	card := f.waitCardTitle(t, "Which color?")
	if res := f.answerCard(t, card, wire.SessionAnswer{Choice: "2"}); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	d.endedWith(t, map[string]string{"Which color?": "blue"})
	d.wantKeys(t, "down", "enter")
}

func TestAskKeys_SingleSelectOtherTypesIntoTheLastRow(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	d := f.openDialog(t, 0, colorQ)
	card := f.waitCardTitle(t, "Which color?")
	if res := f.answerCard(t, card, wire.SessionAnswer{Choice: "other", Text: "teal"}); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	d.endedWith(t, map[string]string{"Which color?": "teal"})
	d.wantKeys(t, append(append([]string{"down", "down"}, chars("teal")...), "enter")...)
}

func TestAskKeys_MultiSelectTogglesThenAdvancesOnTheNextRow(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	d := f.openDialog(t, 0, sizesQ)
	card := f.waitCardTitle(t, "Which sizes?")
	if res := f.answerCard(t, card, wire.SessionAnswer{Choices: []string{"1", "3"}}); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	d.endedWith(t, map[string]string{"Which sizes?": "S, L"})
	d.wantKeys(t, "enter", "down", "down", "enter", "down", "down", "enter")
}

func TestAskKeys_MultiSelectOtherIsTypedNeverEntered(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	d := f.openDialog(t, 0, sizesQ)
	card := f.waitCardTitle(t, "Which sizes?")
	if res := f.answerCard(t, card, wire.SessionAnswer{Choices: []string{"2", "other"}, Text: "XL"}); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	d.endedWith(t, map[string]string{"Which sizes?": "M, XL"})
	// Enter on the typed row would uncheck it: after the text, Down to Next.
	d.wantKeys(t, "down", "enter", "down", "down", "X", "L", "down", "enter")
}

func TestAskKeys_ThreeQuestionsAnswerOneCardEachAndSubmitTheReview(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	d := f.openDialog(t, 0, colorQ, sizesQ, nameQ)
	if res := f.answerCard(t, f.waitCardTitle(t, "Which color?"), wire.SessionAnswer{Choice: "1"}); res.Refused {
		t.Fatalf("q1 refused: %+v", res)
	}
	if res := f.answerCard(t, f.waitCardTitle(t, "Which sizes?"), wire.SessionAnswer{Choices: []string{"2"}}); res.Refused {
		t.Fatalf("q2 refused: %+v", res)
	}
	if res := f.answerCard(t, f.waitCardTitle(t, "Which name?"), wire.SessionAnswer{Choice: "other", Text: "Cy"}); res.Refused {
		t.Fatalf("q3 refused: %+v", res)
	}
	d.endedWith(t, map[string]string{"Which color?": "red", "Which sizes?": "M", "Which name?": "Cy"})
	want := []string{"enter", "down", "enter", "down", "down", "down", "enter", "down", "down", "C", "y", "enter", "enter"}
	d.wantKeys(t, want...)
}

func TestAskKeys_ScreenChangingBetweenKeysStopsWithAnErrorAndLeavesTheDialogOpen(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	d := f.openDialog(t, 1, colorQ)
	card := f.waitCardTitle(t, "Which color?")
	res := f.answerCard(t, card, wire.SessionAnswer{Choice: "2"})
	if !res.Refused || res.Reason != wire.ReasonNotAnswerable || !strings.Contains(res.Message, "dialog is still open") {
		t.Fatalf("result = %+v, want a not-answerable refusal", res)
	}
	d.wantKeys(t, "down")
	if _, err := os.Stat(d.answers); err == nil {
		t.Error("the dialog ended")
	}
	if st := f.info(t).Status; st.State != wire.SessionBlocked {
		t.Errorf("status = %+v, want blocked: the dialog is open", st)
	}
}

func TestAskKeys_StaleCardIdSendsNothing(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	d := f.openDialog(t, 0, colorQ)
	card := f.waitCardTitle(t, "Which color?")
	for _, id := range []string{"x" + card.ID, strings.Replace(card.ID, "-", "-9", 1), card.ID[:len(card.ID)-1] + "0"} {
		res := f.answer(t, wire.SessionAnswerRequest{CardID: id, Answers: []wire.SessionAnswer{{Choice: "2"}}})
		if !res.Refused || res.Reason != wire.ReasonStale {
			t.Errorf("id %q: result = %+v, want stale", id, res)
		}
	}
	d.wantKeys(t)
}

func TestAskKeys_DismissSendsEsc(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	d := f.openDialog(t, 0, colorQ)
	card := f.waitCardTitle(t, "Which color?")
	if res := f.answerCard(t, card, wire.SessionAnswer{}); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	if got := d.ended(t); got != `"cancelled"` {
		t.Errorf("ended = %s, want cancelled", got)
	}
	d.wantKeys(t, "esc")
}

// keys sends named keys through the keys route.
func (f askFixture) keys(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		res, err := f.h.Client.Sessions.Keys(f.ctx, wire.SessionKeysRequest{ID: f.id, Key: n})
		if err != nil || res.Refused {
			t.Fatalf("key %q = %+v, %v", n, res, err)
		}
	}
}

func TestAskKeys_DigitsActPerScreenKind(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	// Single-select: a digit picks and advances at once.
	d := f.openDialog(t, 0, colorQ)
	f.waitCardTitle(t, "Which color?")
	f.keys(t, "2")
	d.endedWith(t, map[string]string{"Which color?": "blue"})

	// Multi-select: a digit toggles and the cursor stays; Next advances.
	f.waitIdle(t)
	d = f.openDialog(t, 0, sizesQ)
	f.waitCardTitle(t, "Which sizes?")
	f.keys(t, "1", "3", "down", "down", "down", "down", "enter")
	d.endedWith(t, map[string]string{"Which sizes?": "S, L"})
}

func TestAskKeys_ReviewScreenShowsACardAndSubmitsOnAnswer(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	d := f.openDialog(t, 0, colorQ, nameQ)
	f.waitCardTitle(t, "Which color?")
	// Answered at the terminal, so gx's own submit never runs.
	f.keys(t, "1", "2")
	card := f.waitCardTitle(t, "Ready to submit your answers?")
	q := card.Questions[0]
	if len(q.Options) != 2 || q.Options[0].Label != "Submit answers" || !strings.Contains(q.Body, "red") {
		t.Fatalf("review card = %+v", q)
	}
	if res := f.answerCard(t, card, wire.SessionAnswer{Choice: "1"}); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	d.endedWith(t, map[string]string{"Which color?": "red", "Which name?": "Bob"})
}
