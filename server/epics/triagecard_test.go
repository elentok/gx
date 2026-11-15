package epics_test

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/epics"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
)

func TestPlaceTriage(t *testing.T) {
	t.Parallel()
	row := func(w triage.WriteState) triage.Row { return triage.Row{Write: w} }
	verdict := triage.Row{Verdict: "valid"}
	q := func(id string) triage.Question { return triage.Question{ID: id, Text: id, Kind: triage.QuestionAsk} }
	rec := func(step string, complete bool, rows ...triage.Row) *triage.Record {
		return &triage.Record{State: triage.StateLive, Step: step, LastPassComplete: complete, Rows: rows}
	}
	// questions is a Questions record with the given rounds; an unplaced ask
	// question on a row means the agent has more to print.
	questions := func(rounds []triage.Round, rows ...triage.Row) *triage.Record {
		r := rec(tickets.StepQuestions, true, rows...)
		r.Rounds = rounds
		return r
	}
	round := triage.Round{Questions: []triage.Question{q("a"), q("b"), q("c")}, Answers: map[string]string{"a": "x"}}
	answered := triage.Round{Questions: []triage.Question{q("a")}, Answers: map[string]string{"a": "x"}}
	const link = "#/triage/proj"
	for _, tc := range []struct {
		name    string
		rec     *triage.Record
		session bool
		section wire.CardSection
		pill    string
		line    string
		retry   int
		stopped bool
		kind    wire.CardActionKind
		label   string
		link    string
	}{
		{"verify, session", rec(tickets.StepVerify, false, verdict, triage.Row{}, triage.Row{}), true, wire.SectionDesigning, "Verify", "Verifying 1 of 3", 0, false, wire.CardActionContinue, "Continue", ""},
		{"verify, no session", rec(tickets.StepVerify, false, verdict), false, wire.SectionDesigning, "Verify", "Verifying 1 of 1", 0, true, wire.CardActionContinue, "Continue", ""},
		{"route, session", rec(tickets.StepRoute, true), true, wire.SectionDesigning, "Route", "Routing", 0, false, wire.CardActionContinue, "Continue", ""},
		{"questions, round waits, session", questions([]triage.Round{round}), true, wire.SectionNeedsYou, "Needs you", "Round 1 · 2 questions left", 0, false, wire.CardActionOpen, "Answer", link},
		{"questions, second round waits, no session", questions([]triage.Round{answered, round}), false, wire.SectionNeedsYou, "Needs you", "Round 2 · 2 questions left", 0, false, wire.CardActionOpen, "Answer", link},
		{"questions, final card, session", questions([]triage.Round{answered}), true, wire.SectionNeedsYou, "Needs you", "Write this plan?", 0, false, wire.CardActionOpen, "Review plan", link},
		{"questions, final card, no session", questions(nil), false, wire.SectionNeedsYou, "Needs you", "Write this plan?", 0, false, wire.CardActionOpen, "Review plan", link},
		{"questions, agent still printing", questions([]triage.Round{answered}, triage.Row{Questions: []triage.Question{q("z")}}), true, wire.SectionDesigning, "Questions", "Questions", 0, false, wire.CardActionContinue, "Continue", ""},
		{"write, failed row, session", rec(tickets.StepWrite, true, row(triage.WriteFailed), row(triage.WriteDone)), true, wire.SectionDesigning, "Write", "1 of 2 written", 1, false, wire.CardActionContinue, "Continue", ""},
		{"write, failed row, no session", rec(tickets.StepWrite, true, row(triage.WriteFailed), row(triage.WriteDone)), false, wire.SectionNeedsYou, "Write", "1 of 2 written", 1, false, wire.CardActionContinue, "Continue", ""},
		{"write, pending row, no session", rec(tickets.StepWrite, true, row(triage.WriteFailed), row(triage.WritePending)), false, wire.SectionDesigning, "Write", "0 of 2 written", 1, true, wire.CardActionContinue, "Continue", ""},
		{"write, last pass incomplete", rec(tickets.StepWrite, false, row(triage.WriteDone)), false, wire.SectionDesigning, "Write", "1 of 1 written", 0, true, wire.CardActionContinue, "Continue", ""},
		{"abandoned", &triage.Record{State: triage.StateAbandoned, Step: tickets.StepPlan}, false, wire.SectionReadyToDesign, "", "", 0, false, wire.CardActionTriage, "Triage", ""},
		{"done", &triage.Record{State: triage.StateDone, Step: tickets.StepWrite}, false, wire.SectionReadyToDesign, "", "", 0, false, wire.CardActionTriage, "Triage", ""},
		{"none", nil, false, wire.SectionReadyToDesign, "", "", 0, false, wire.CardActionTriage, "Triage", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := epics.PlaceTriage("proj", tc.rec, tc.session)
			if got.Section != tc.section || got.Pill != tc.pill || got.Line != tc.line || got.RetryFailed != tc.retry ||
				got.Stopped != tc.stopped || got.Action.Kind != tc.kind ||
				got.Action.Label != tc.label || got.Action.Link != tc.link || got.Action.Target != "proj" {
				t.Errorf("placement = %+v", got)
			}
		})
	}
}

func TestEpicList_FollowUpsCardFollowsTheTriageLifetime(t *testing.T) {
	t.Parallel()
	var rs *triage.Store
	var projDir string
	h, base := rigs.StartEpicDetail(t, func(store string) {
		loop := "---\nstatus: open\n---\n"
		rigs.WriteEpicFiles(t, store, "follow-ups", loop, map[string]string{
			"01-a.md": rigs.IssueMD("01", "open", "category: bug\n", "A"),
			"02-b.md": rigs.IssueMD("02", "open", "category: idea\n", "B"),
		})
		projDir = filepath.Join(store, "proj")
		rs = triage.NewStore(projDir)
	})
	card := func() (wire.EpicListRow, bool) {
		t.Helper()
		code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
		for _, r := range rigs.GetEpicList(t, code, body).Epics {
			if r.Name == "follow-ups" {
				return r, true
			}
		}
		return wire.EpicListRow{}, false
	}
	// setStatus rewrites the two follow-ups with the given statuses.
	setStatus := func(a, b string) {
		t.Helper()
		rigs.WriteEpicFiles(t, filepath.Dir(projDir), "follow-ups", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": rigs.IssueMD("01", a, "category: bug\n", "A"),
			"02-b.md": rigs.IssueMD("02", b, "category: idea\n", "B"),
		})
	}

	r, ok := card()
	if !ok || r.StatusWords != "1 bug, 1 idea" || r.StatePill != "" {
		t.Fatalf("before any Triage: %+v, %v", r, ok)
	}
	rec, err := rs.Create()
	if err != nil {
		t.Fatal(err)
	}
	rec.Step = tickets.StepWrite
	rec.LastPassComplete = true
	if err := rs.Save(rec); err != nil {
		t.Fatal(err)
	}
	r, _ = card()
	if r.StatePill != "Write" || r.StepLine != "0 of 2 written" || r.Section != wire.SectionDesigning || r.Actions[0].Kind != wire.CardActionContinue {
		t.Errorf("live Triage, no session: %+v", r)
	}

	// Every row written: the follow-ups are gone from the open count, the card stays.
	setStatus("done", "done")
	if !servertest.Poll(5e9, func() bool { r, _ = card(); return r.StatusWords == "0 left" }) {
		t.Fatalf("live Triage at zero open: %+v", r)
	}

	rec, _ = rs.LoadLive()
	rec.State, rec.Step = triage.StateDone, "done"
	if err := rs.Save(rec); err != nil {
		t.Fatal(err)
	}
	if r, ok := card(); ok {
		t.Errorf("done at zero open: card = %+v, want none", r)
	}

	setStatus("done", "open")
	if !servertest.Poll(5e9, func() bool { r, ok = card(); return ok }) {
		t.Fatal("done with a kept row: no card")
	}
	if r.StatusWords != "1 idea" || r.StatePill != "" || r.Actions[0].Kind != wire.CardActionTriage {
		t.Errorf("done with a kept row: %+v", r)
	}

	// A ready epic that is not queued: the Done card; queued: the plain card again.
	rigs.WriteEpicFiles(t, filepath.Dir(projDir), "triage-ready", "---\nstatus: open\n---\n", map[string]string{
		"01-a.md": rigs.IssueMD("01", "open", "", "A"),
		"02-b.md": rigs.IssueMD("02", "open", "", "B"),
	})
	hist, err := rs.History()
	if err != nil || len(hist) == 0 {
		t.Fatalf("history = %v, %v", hist, err)
	}
	rec = hist[0]
	rec.ReadyEpic = "proj:triage-ready"
	if err := rs.Save(rec); err != nil {
		t.Fatal(err)
	}
	if !servertest.Poll(5e9, func() bool { r, _ = card(); return r.StatePill == "Done" }) {
		t.Fatalf("done with an unqueued ready epic: %+v", r)
	}
	if r.StepLine != "2 ready to queue" || r.Actions[0].Kind != wire.CardActionQueue || r.Actions[0].Label != "Queue" || r.Actions[0].Target != "proj:triage-ready" {
		t.Errorf("done card: %+v", r)
	}
	if res, err := h.Client.Queue.Add(t.Context(), "proj:triage-ready", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	if !servertest.Poll(5e9, func() bool { r, _ = card(); return r.StatePill == "" }) || r.Actions[0].Kind != wire.CardActionTriage {
		t.Errorf("done with a queued ready epic: %+v", r)
	}
}
