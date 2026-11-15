package designflows

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake/triageplay"
	"github.com/elentok/gx/testutil/gxfixture/spec"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
)

type triageWant struct {
	shape string
	// section is the Follow-ups card's; empty means no card.
	section wire.CardSection
	pill    string
	words   string
	triage  wire.FollowUpTriage
	step    string // current step of `server triage show`, empty without a Triage
	state   wire.TriageState
	// labels are the card's action labels in order; nil skips the check.
	labels []string
}

func TestTriageScenario_EveryShapeShowsTheCardItNames(t *testing.T) {
	t.Parallel()
	for _, w := range []triageWant{
		{shape: "c-zero", triage: ""},
		{shape: "c-few", section: wire.SectionReadyToDesign, words: "1 bug, 1 idea", triage: wire.FollowUpTriageNone},
		{shape: "c-many", section: wire.SectionReadyToDesign, words: "30 bugs, 15 ideas", triage: wire.FollowUpTriageNone},
		{shape: "c-live", section: wire.SectionNeedsYou, pill: "Needs you", words: "6 bugs, 6 ideas", triage: wire.FollowUpTriageWaiting, step: "questions", state: wire.TriageLive},
		{shape: "verify", section: wire.SectionDesigning, pill: "Verify", words: "30 bugs, 15 ideas", triage: wire.FollowUpTriageRunning, step: "verify", state: wire.TriageLive},
		{shape: "route", section: wire.SectionNeedsYou, pill: "Needs you", words: "4 bugs, 2 ideas", triage: wire.FollowUpTriageWaiting, step: "questions", state: wire.TriageLive},
		{shape: "route-12-areas", section: wire.SectionNeedsYou, pill: "Needs you", words: "4 bugs, 2 ideas", triage: wire.FollowUpTriageWaiting, step: "questions", state: wire.TriageLive},
		{shape: "route-no-area", section: wire.SectionNeedsYou, pill: "Needs you", words: "4 bugs, 2 ideas", triage: wire.FollowUpTriageWaiting, step: "questions", state: wire.TriageLive},
		{shape: "questions", section: wire.SectionNeedsYou, pill: "Needs you", words: "4 bugs, 2 ideas", triage: wire.FollowUpTriageWaiting, step: "questions", state: wire.TriageLive},
		{shape: "final-approval", section: wire.SectionNeedsYou, pill: "Needs you", words: "4 bugs, 2 ideas", triage: wire.FollowUpTriageWaiting, step: "questions", state: wire.TriageLive},
		{shape: "writing", section: wire.SectionDesigning, pill: "Write", words: "6 bugs", triage: wire.FollowUpTriageRunning, step: "write", state: wire.TriageLive, labels: []string{"Retry failed (2)", "Continue"}},
		{shape: "result", section: wire.SectionReadyToDesign, pill: "Done", words: "2 bugs", triage: wire.FollowUpTriageNone, state: wire.TriageDone, labels: []string{"Queue"}},
		{shape: "stopped", section: wire.SectionDesigning, pill: "Route", words: "4 bugs", triage: wire.FollowUpTriageStopped, step: "route", state: wire.TriageLive},
		{shape: "journey", section: wire.SectionReadyToDesign, words: "3 bugs, 2 ideas", triage: wire.FollowUpTriageNone},
	} {
		t.Run(w.shape, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "triage", nil, "--shape", w.shape)
			var list wire.EpicList
			var card *wire.EpicListRow
			var count *wire.FollowUpCount
			ok := testutil.Poll(40*time.Second, 500*time.Millisecond, func() bool {
				list, card, count = r.epicList(t), nil, nil
				for i, c := range list.Epics {
					if c.CardKind == wire.CardKindFollowUps && c.Project == project {
						card = &list.Epics[i]
					}
				}
				for i, c := range list.FollowUps {
					if c.Project == project {
						count = &list.FollowUps[i]
					}
				}
				if w.section == "" {
					return card == nil && count == nil
				}
				return card != nil && count != nil && card.Section == w.section && card.StatePill == w.pill && count.Triage == w.triage
			})
			if !ok {
				t.Fatalf("follow-ups card %+v, count %+v; want section %q pill %q triage %q", card, count, w.section, w.pill, w.triage)
			}
			if w.section == "" {
				return
			}
			if card.StatusWords != w.words {
				t.Errorf("words = %q, want %q", card.StatusWords, w.words)
			}
			if w.labels != nil {
				var got []string
				for _, a := range card.Actions {
					got = append(got, a.Label)
				}
				if !slices.Equal(got, w.labels) {
					t.Errorf("action labels = %q, want %q", got, w.labels)
				}
			}
			if w.state == "" {
				return
			}
			if v := r.triageView(t); v.State != w.state || (w.step != "" && currentStep(v) != w.step) {
				t.Errorf("triage state %q step %q, want %q and %q", v.State, currentStep(v), w.state, w.step)
			}
		})
	}
}

// The area-count variants of route differ in the new-area card: three options
// with some area, two with none, and none at all (a decided filing) at 12.
func TestTriageScenario_RouteVariantsShowTheNewAreaCardTheSpecNames(t *testing.T) {
	t.Parallel()
	for _, w := range []struct {
		shape   string
		options int // of the first question of round 1; zero: no new-area question
		decided string
	}{
		{"route", 3, ""},
		{"route-no-area", 2, ""},
		{"route-12-areas", 0, "Which area does it go to?"},
	} {
		t.Run(w.shape, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "triage", nil, "--shape", w.shape)
			var rec struct {
				Rounds  []triage.Round   `json:"rounds"`
				Decided []triage.Decided `json:"decided"`
			}
			if !testutil.Poll(40*time.Second, 500*time.Millisecond, func() bool {
				rec.Rounds, rec.Decided = nil, nil
				return json.Unmarshal([]byte(r.Gx(t, "tickets", "triage", "show", "--project", project, "--json")), &rec) == nil && len(rec.Rounds) > 0
			}) {
				t.Fatal("no round was saved")
			}
			first := rec.Rounds[0].Questions[0]
			if got := first.ID == triageplay.NewAreaQuestionID; got != (w.options > 0) || (got && len(first.Options) != w.options) {
				t.Errorf("first question %q with options %v, want a new-area question of %d options (0: none)", first.ID, first.Options, w.options)
			}
			if w.decided != "" && !slices.ContainsFunc(rec.Decided, func(d triage.Decided) bool { return d.Question == w.decided }) {
				t.Errorf("decided = %+v, want %q", rec.Decided, w.decided)
			}
		})
	}
}

func currentStep(v wire.TriageView) string {
	for _, s := range v.Steps {
		if s.State == wire.MapStepCurrent {
			return string(s.Step)
		}
	}
	return ""
}

func (r *running) epicList(t *testing.T) wire.EpicList {
	t.Helper()
	var list wire.EpicList
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "epics", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func (r *running) triageView(t *testing.T) wire.TriageView {
	t.Helper()
	var v wire.TriageView
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "triage", "show", project, "--json")), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// followUpsOf polls the follow-up counts until two projects show and returns
// pick of each, sorted.
func followUpsOf[T cmp.Ordered](t *testing.T, r *running, shape string, pick func(wire.FollowUpCount) (T, bool)) []T {
	t.Helper()
	var got []T
	if !testutil.Poll(40*time.Second, 500*time.Millisecond, func() bool {
		got = nil
		for _, c := range r.epicList(t).FollowUps {
			if v, ok := pick(c); ok {
				got = append(got, v)
			}
		}
		slices.Sort(got)
		return len(got) == 2
	}) {
		t.Fatalf("%s: follow-up counts = %v, want two projects", shape, got)
	}
	return got
}

func TestTriageScenario_CNamedListsTheWorkspacesTwoProjects(t *testing.T) {
	t.Parallel()
	got := followUpsOf(t, startWith(t, "triage", nil, "--shape", "c-named"), "c-named", func(c wire.FollowUpCount) (string, bool) {
		return c.Project, slices.Equal(c.Workspaces, []string{spec.TriageWorkspace})
	})
	if want := []string{project, spec.TriageSide}; !slices.Equal(got, want) {
		t.Errorf("projects = %v, want %v", got, want)
	}
}

func TestTriageScenario_CFewSpansTwoProjects(t *testing.T) {
	t.Parallel()
	got := followUpsOf(t, startWith(t, "triage", nil, "--shape", "c-few"), "c-few", func(c wire.FollowUpCount) (int, bool) { return c.Open, true })
	if !slices.Equal(got, []int{1, 2}) {
		t.Errorf("open counts = %v, want [1 2]", got)
	}
}

// waitTriage polls the shown Triage until ok holds and returns that view.
func (r *running) waitTriage(t *testing.T, what string, ok func(v wire.TriageView) bool) wire.TriageView {
	t.Helper()
	var v wire.TriageView
	if !testutil.Poll(60*time.Second, 500*time.Millisecond, func() bool {
		v = r.triageView(t)
		return ok(v)
	}) {
		t.Fatalf("waiting for %s: state %q step %q, %d open questions, rows %+v", what, v.State, currentStep(v), v.OpenQuestions, v.Rows)
	}
	return v
}

// waitNoSession polls until none of the project's Triage sessions is working.
// The card cannot say: at Approve it reads "waiting for you" while the Plan
// session still prints its run-end line.
func (r *running) waitNoSession(t *testing.T) {
	t.Helper()
	if !testutil.Poll(60*time.Second, 250*time.Millisecond, func() bool {
		for _, s := range r.sessionsOf(t) {
			if s.Flow == string(tickets.FlowTriage) && s.FlowKey == project && s.Status.State == wire.SessionWorking {
				return false
			}
		}
		return true
	}) {
		t.Fatal("the Triage session never ended")
	}
}

func TestTriageScenario_JourneyRunsFromStartToDoneWithOneRetry(t *testing.T) {
	t.Parallel()
	r := startWith(t, "triage", nil, "--shape", "journey", "--auto-advance-delay", "100ms")
	r.Gx(t, "server", "triage", "open", project)

	// Verify takes two sessions: the fake saves three verdicts per session.
	verified := func(v wire.TriageView) int {
		n := 0
		for _, row := range v.Rows {
			if row.Verdict != "" {
				n++
			}
		}
		return n
	}
	r.waitTriage(t, "the first Verify batch", func(v wire.TriageView) bool { return verified(v) == 3 })
	v := r.waitTriage(t, "Route to end at Questions", func(v wire.TriageView) bool { return currentStep(v) == "questions" && v.OpenQuestions == 2 })
	if verified(v) != 5 || len(v.ActionCounts) != 5 {
		t.Errorf("at Approve: %d verdicts and action groups %v, want 5 and all five actions", verified(v), v.ActionCounts)
	}

	// Answer every open question, then approve. Plan's session prints its run-end
	// line after the last save: a person approving before it ends would also
	// see the countdown advance that session into Write.
	r.waitNoSession(t)
	for _, row := range v.Rows {
		var args []string
		for _, q := range row.Questions {
			answer := "ok" // a design question takes any text; a quick one needs one of its options
			if len(q.Options) > 0 {
				answer = q.Options[0]
			}
			args = append(args, "--answer", q.ID+"="+answer)
		}
		if len(args) == 0 {
			continue
		}
		r.Gx(t, append([]string{"tickets", "triage", "row", "--project", project, "--row", row.Address, "--revision", fmt.Sprint(r.triageView(t).Revision)}, args...)...)
	}
	v = r.waitTriage(t, "no open question", func(v wire.TriageView) bool { return v.OpenQuestions == 0 })
	r.Gx(t, "tickets", "triage", "approve", "--project", project, "--revision", fmt.Sprint(v.Revision))
	// The CLI approve starts nothing: the chat runs Write itself, so open a session at step write.
	r.Gx(t, "server", "triage", "open", project, "--new")

	// The Write session leaves the scripted rows failed and the rest done: the
	// first fails before its move and the second after it. The fifth is a keep
	// in the plan, so its scripted failure never fires.
	failedRows := map[string]string{
		project + ":follow-ups/01": triageplay.RefusalBeforeMove,
		project + ":follow-ups/02": triageplay.RefusalAfterMove,
	}
	v = r.waitTriage(t, "the Write session to end", func(v wire.TriageView) bool {
		failed, written := 0, 0
		for _, row := range v.Rows {
			switch row.WriteState {
			case "failed":
				failed++
			case "done":
				written++
			}
		}
		return failed == len(failedRows) && written == 3
	})
	// The last pass saves after the last row, so wait for the session to end
	// before retrying: a Retry while one is live starts no new session.
	r.waitNoSession(t)
	v = r.triageView(t)
	for _, row := range v.Rows {
		if want, scripted := failedRows[row.Address]; row.WriteState == "failed" && (!scripted || row.Refusal != want) {
			t.Errorf("failed row %s with refusal %q, want one of %v", row.Address, row.Refusal, failedRows)
		}
	}

	// Each Retry runs its own Write session; the Triage is done after the last.
	for address := range failedRows {
		r.Gx(t, "server", "triage", "retry", project, address, "--revision", fmt.Sprint(r.triageView(t).Revision))
		r.waitTriage(t, "the retry of "+address, func(v wire.TriageView) bool {
			for _, row := range v.Rows {
				if row.Address == address {
					return row.WriteState == "done"
				}
			}
			return false
		})
		r.waitNoSession(t)
	}
	v = r.waitTriage(t, "the Triage to be done", func(v wire.TriageView) bool { return v.State == wire.TriageDone })
	if v.Result == nil || v.Result.ReadyEpic != project+":triage-ready" {
		t.Fatalf("result = %+v, want a ready epic", v.Result)
	}
}
