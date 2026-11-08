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
	rec := func(step string, complete bool, rows ...triage.Row) *triage.Record {
		return &triage.Record{State: triage.StateLive, Step: step, LastPassComplete: complete, Rows: rows}
	}
	for _, tc := range []struct {
		name    string
		rec     *triage.Record
		session bool
		section wire.CardSection
		pill    string
		kind    wire.CardActionKind
		label   string
		link    string
	}{
		{"verify, session", rec(tickets.StepVerify, false), true, wire.SectionDesigning, "", wire.CardActionOpen, "Open", "#/triage/proj"},
		{"verify, no session", rec(tickets.StepVerify, false), false, wire.SectionDesigning, epics.PillStopped, wire.CardActionContinue, "Continue", ""},
		{"plan, session", rec(tickets.StepPlan, true), true, wire.SectionDesigning, "", wire.CardActionOpen, "Open", "#/triage/proj"},
		{"plan, no session", rec(tickets.StepPlan, true), false, wire.SectionDesigning, epics.PillStopped, wire.CardActionContinue, "Continue", ""},
		{"approve, session", rec(tickets.StepApprove, true), true, wire.SectionNeedsYou, epics.PillWaitingForYou, wire.CardActionOpen, "Review plan", "#/triage/proj"},
		{"approve, no session", rec(tickets.StepApprove, true), false, wire.SectionNeedsYou, epics.PillWaitingForYou, wire.CardActionOpen, "Review plan", "#/triage/proj"},
		{"write, failed row, session", rec(tickets.StepWrite, true, row(triage.WriteFailed)), true, wire.SectionDesigning, "", wire.CardActionOpen, "Open", "#/triage/proj"},
		{"write, failed row, no session", rec(tickets.StepWrite, true, row(triage.WriteFailed), row(triage.WriteDone)), false, wire.SectionNeedsYou, epics.PillWaitingForYou, wire.CardActionOpen, "Review failures", "#/triage/proj"},
		{"write, pending row, no session", rec(tickets.StepWrite, true, row(triage.WriteFailed), row(triage.WritePending)), false, wire.SectionDesigning, epics.PillStopped, wire.CardActionContinue, "Continue", ""},
		{"write, last pass incomplete", rec(tickets.StepWrite, false, row(triage.WriteDone)), false, wire.SectionDesigning, epics.PillStopped, wire.CardActionContinue, "Continue", ""},
		{"abandoned", &triage.Record{State: triage.StateAbandoned, Step: tickets.StepPlan}, false, wire.SectionReadyToDesign, "", wire.CardActionTriage, "Triage", ""},
		{"done", &triage.Record{State: triage.StateDone, Step: tickets.StepWrite}, false, wire.SectionReadyToDesign, "", wire.CardActionTriage, "Triage", ""},
		{"none", nil, false, wire.SectionReadyToDesign, "", wire.CardActionTriage, "Triage", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := epics.PlaceTriage("proj", tc.rec, tc.session)
			if got.Section != tc.section || got.Pill != tc.pill || got.Action.Kind != tc.kind ||
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
	if r.StatePill != epics.PillStopped || r.Section != wire.SectionDesigning || r.Actions[0].Kind != wire.CardActionContinue {
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
}
