package main

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

type triageWant struct {
	shape string
	// card is the Follow-ups card of project demo; empty section means no card.
	section wire.CardSection
	pill    string
	words   string
	triage  wire.FollowUpTriage
	// step is the current step of `server triage show`, empty when there is no Triage.
	step string
	// state is the shown Triage state, empty when none.
	state wire.TriageState
}

func TestTriageScenario_EveryShapeShowsTheCardItNames(t *testing.T) {
	t.Parallel()
	for _, w := range []triageWant{
		{shape: "c-zero", triage: ""},
		{shape: "c-few", section: wire.SectionReadyToDesign, words: "1 bug, 1 idea", triage: wire.FollowUpTriageNone},
		{shape: "c-many", section: wire.SectionReadyToDesign, words: "30 bugs, 15 ideas", triage: wire.FollowUpTriageNone},
		{shape: "c-live", section: wire.SectionNeedsYou, pill: "Waiting for you", words: "6 bugs, 6 ideas", triage: wire.FollowUpTriageWaiting, step: "approve", state: wire.TriageLive},
		{shape: "verify", section: wire.SectionDesigning, words: "30 bugs, 15 ideas", triage: wire.FollowUpTriageRunning, step: "verify", state: wire.TriageLive},
		{shape: "b-questions", section: wire.SectionNeedsYou, pill: "Waiting for you", words: "3 bugs, 3 ideas", triage: wire.FollowUpTriageWaiting, step: "approve", state: wire.TriageLive},
		{shape: "b-review", section: wire.SectionNeedsYou, pill: "Waiting for you", words: "5 bugs, 5 ideas", triage: wire.FollowUpTriageWaiting, step: "approve", state: wire.TriageLive},
		{shape: "writing", section: wire.SectionDesigning, words: "6 bugs", triage: wire.FollowUpTriageRunning, step: "write", state: wire.TriageLive},
		{shape: "result", section: wire.SectionReadyToDesign, words: "2 bugs", triage: wire.FollowUpTriageNone, state: wire.TriageDone},
		{shape: "stopped", section: wire.SectionDesigning, pill: "Stopped", words: "4 bugs", triage: wire.FollowUpTriageStopped, step: "plan", state: wire.TriageLive},
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
			if w.state == "" {
				return
			}
			v := r.triageView(t)
			if v.State != w.state {
				t.Errorf("triage state = %q, want %q", v.State, w.state)
			}
			if w.step != "" {
				if cur := currentStep(v); cur != w.step {
					t.Errorf("triage step = %q, want %q", cur, w.step)
				}
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
	if err := json.Unmarshal([]byte(r.gx(t, "server", "epics", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func (r *running) triageView(t *testing.T) wire.TriageView {
	t.Helper()
	var v wire.TriageView
	if err := json.Unmarshal([]byte(r.gx(t, "server", "triage", "show", project, "--json")), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestTriageScenario_CManyCountsFortyFiveOpenFollowUps(t *testing.T) {
	t.Parallel()
	r := startWith(t, "triage", nil, "--shape", "c-many")
	var count wire.FollowUpCount
	if !testutil.Poll(40*time.Second, 500*time.Millisecond, func() bool {
		for _, c := range r.epicList(t).FollowUps {
			count = c
		}
		return count.Open == 45
	}) {
		t.Fatalf("open follow-ups = %d, want 45", count.Open)
	}
	if count.Bugs != 30 || count.Ideas != 15 {
		t.Errorf("split = %d bugs, %d ideas, want 30 and 15", count.Bugs, count.Ideas)
	}
}

func TestTriageScenario_CNamedListsTheWorkspacesTwoProjects(t *testing.T) {
	t.Parallel()
	r := startWith(t, "triage", nil, "--shape", "c-named")
	var got []string
	if !testutil.Poll(40*time.Second, 500*time.Millisecond, func() bool {
		got = nil
		for _, c := range r.epicList(t).FollowUps {
			if slices.Equal(c.Workspaces, []string{spec.TriageWorkspace}) {
				got = append(got, c.Project)
			}
		}
		slices.Sort(got)
		return len(got) == 2
	}) {
		t.Fatalf("projects in %q: %v, want two", spec.TriageWorkspace, got)
	}
	if want := []string{project, spec.TriageSide}; !slices.Equal(got, want) {
		t.Errorf("projects = %v, want %v", got, want)
	}
}

func TestTriageScenario_CFewSpansTwoProjects(t *testing.T) {
	t.Parallel()
	r := startWith(t, "triage", nil, "--shape", "c-few")
	var open []int
	if !testutil.Poll(40*time.Second, 500*time.Millisecond, func() bool {
		open = nil
		for _, c := range r.epicList(t).FollowUps {
			open = append(open, c.Open)
		}
		return len(open) == 2
	}) {
		t.Fatalf("follow-up counts = %v, want two projects", open)
	}
	slices.Sort(open)
	if !slices.Equal(open, []int{1, 2}) {
		t.Errorf("open counts = %v, want [1 2]", open)
	}
}

func TestTriageScenario_ApproveShapesCarryQuestionsAndChanges(t *testing.T) {
	t.Parallel()
	for shape, check := range map[string]func(t *testing.T, v wire.TriageView){
		"b-questions": func(t *testing.T, v wire.TriageView) {
			if v.OpenQuestions != 3 {
				t.Errorf("open questions = %d, want 3", v.OpenQuestions)
			}
		},
		"b-review": func(t *testing.T, v wire.TriageView) {
			if v.OpenQuestions != 0 || v.ChangedRows != 3 {
				t.Errorf("open questions %d, changed rows %d, want 0 and 3", v.OpenQuestions, v.ChangedRows)
			}
			if len(v.ActionCounts) != 5 {
				t.Errorf("action groups = %v, want five", v.ActionCounts)
			}
		},
		"writing": func(t *testing.T, v wire.TriageView) {
			var failed []string
			for _, row := range v.Rows {
				if row.WriteState == "failed" {
					failed = append(failed, row.NewAddress)
				}
			}
			if len(failed) != 2 || (failed[0] == "") == (failed[1] == "") {
				t.Errorf("failed rows' new addresses = %q, want one empty and one set", failed)
			}
		},
		"result": func(t *testing.T, v wire.TriageView) {
			if v.Result == nil || v.Result.ReadyEpic != project+":triage-ready" {
				t.Fatalf("result = %+v, want a ready epic", v.Result)
			}
			for _, g := range v.Result.Groups {
				if g.Action == "keep" && len(g.Items) != 2 {
					t.Errorf("kept rows = %d, want 2", len(g.Items))
				}
			}
		},
	} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "triage", nil, "--shape", shape)
			check(t, r.triageView(t))
		})
	}
}
