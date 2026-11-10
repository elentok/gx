package prompt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

var planOptions = []string{"Yes, and use auto mode", "Yes, manually approve edits", "Tell Claude what to change"}

// planFile writes a plan under a .claude/plans dir and returns its path.
func planFile(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".claude", "plans", name+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPlanCard_CarriesThePlanFileTextAndAnIdPerPlan(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.PlanDialog(planFile(t, "plan-one", "# One\n- step full text"), planOptions...))
	one := f.permissionCard(t)
	if body := one.Questions[0].Body; !strings.Contains(body, "step full text") {
		t.Errorf("body = %q, want the plan file text", body)
	}

	g := startPromptSessionWithGrace(t, 10*time.Second)
	g.blockOn(t, agentfake.PlanDialog(planFile(t, "plan-two", "# Two"), planOptions...))
	if two := g.permissionCard(t); two.ID == one.ID {
		t.Errorf("both plans have id %q", one.ID)
	}
}

func TestPlanCard_TextRowIsFoundByLabelAndRefusesEmptyText(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.PlanDialog(planFile(t, "plan-x", "# X"), "Tell Claude what to change", "Yes, and use auto mode"))
	card := f.permissionCard(t)
	q := card.Questions[0]
	if !q.Options[0].Text || q.Options[1].Text {
		t.Fatalf("options = %+v, want only the first a text row", q.Options)
	}
	if res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: card.ID, Answers: []wire.SessionAnswer{{Choice: "1"}}}); !res.Refused || res.Reason != wire.ReasonSessionBadRequest {
		t.Fatalf("empty text: %+v, want %s", res, wire.ReasonSessionBadRequest)
	}
	f.nothingSent(t)
}

func TestPlanCard_TextRowAnswerTypesIntoTheBox(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.PlanDialog(planFile(t, "plan-y", "# Y"), "Yes, and use auto mode", "Tell Claude what to change"))
	card := f.permissionCard(t)
	res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: card.ID, Answers: []wire.SessionAnswer{{Choice: "2", Text: "use less steps"}}})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	// The fake logs typed text only when Enter submits it.
	waitForSubmitted(t, f.conv, `"2"`+"\n"+`"use less steps"`+"\n")
}

func TestPlanCard_WithoutTheLabelHasNoTextRow(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.PlanDialog(planFile(t, "plan-z", "# Z"), "Yes, and use auto mode", "No, keep planning"))
	for _, o := range f.permissionCard(t).Questions[0].Options {
		if o.Text {
			t.Errorf("option %+v is a text row", o)
		}
	}
}

func TestPlanCard_DismissSendsEsc(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.PlanDialog(planFile(t, "plan-d", "# D"), planOptions...))
	card := f.permissionCard(t)
	if res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: card.ID}); res.Refused {
		t.Fatalf("dismiss = %+v", res)
	}
	waitForSubmitted(t, f.conv, escLogged)
}

// The card id is the plan file name plus a hash of the dialog and the plan
// text, so an id from before the file or the block changed reaches nothing.
func TestPlanCard_OldIdIsStaleAndSendsNoKeys(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.PlanDialog(planFile(t, "plan-now", "# Now"), planOptions...))
	card := f.permissionCard(t)
	cut := strings.LastIndex(card.ID, "-")
	if cut < 0 {
		t.Fatalf("card id %q has no hash part", card.ID)
	}
	for label, id := range map[string]string{
		"file name changed":  "plan-before" + card.ID[cut:],
		"block hash changed": card.ID[:cut+1] + "000000000000",
	} {
		res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: id, Answers: []wire.SessionAnswer{{Choice: "1"}}})
		if !res.Refused || res.Reason != wire.ReasonStale {
			t.Errorf("%s: answer = %+v, want %s", label, res, wire.ReasonStale)
		}
	}
	f.nothingSent(t)
}

func TestPlanCard_UnreadablePlanFileListsOptionsOnly(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.PlanDialog("/nowhere/.claude/plans/missing.md", planOptions...))
	q := f.permissionCard(t).Questions[0]
	if q.Body != "" || len(q.Options) != 3 {
		t.Errorf("question = %+v, want options and no body", q)
	}
}
