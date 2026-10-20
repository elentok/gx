package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// parkEnvZeroCommit parks ticket 01 of the answer env as zero-commit and returns
// the state token of its question.
func parkEnvZeroCommit(t *testing.T, e answerEnv) string {
	t.Helper()
	if err := e.h.Server.ParkAs(answerAddr, events.ZeroCommit, "no commits"); err != nil {
		t.Fatal(err)
	}
	return stateOf(t, e.ticket)
}

func stateOf(t *testing.T, ticketPath string) string {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	open := tickets.OpenQuestion(evs)
	if open == nil {
		t.Fatal("no open question")
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d:%s", open.Seq, tk.Status)
}

func webUnpark(t *testing.T, h *servertest.Harness, req server.RepairRequest) server.RepairResult {
	t.Helper()
	req.Address = answerAddr
	res, err := h.Server.UnparkAsWeb(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func unparkResult(t *testing.T, res server.RepairResult) repair.UnparkResult {
	t.Helper()
	if res.Refused {
		t.Fatalf("unpark refused: %+v", res)
	}
	var out repair.UnparkResult
	if err := json.Unmarshal(res.Data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func answerEvents(t *testing.T, ticketPath string) []tickets.LogEvent {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	var out []tickets.LogEvent
	for _, ev := range evs {
		if ev.Type == tickets.EventParkAnswer {
			out = append(out, ev)
		}
	}
	return out
}

func TestWebUnpark_MissingOrStaleTokenRefusesBeforeAnyChange(t *testing.T) {
	e := startAnswerEnv(t, nil)
	parkEnvZeroCommit(t, e)

	for name, state := range map[string]string{"missing": "", "stale": "99:needs-answer"} {
		res := webUnpark(t, e.h, server.RepairRequest{State: state})
		if !res.Refused || res.Reason != server.ReasonStale {
			t.Fatalf("%s: unpark = %+v; want refusal %s", name, res, server.ReasonStale)
		}
	}
	if tk, err := schema.ParseTicket(e.ticket); err != nil || tk.Status != schema.StatusNeedsAnswer {
		t.Errorf("ticket = %+v, %v; want still needs-answer", tk, err)
	}
	if got := answerEvents(t, e.ticket); len(got) != 0 {
		t.Errorf("refusal logged %+v", got)
	}
	if got := e.fake.Prompts(answerLabel); len(got) != 0 {
		t.Errorf("refusal prompted the agent: %q", got)
	}
}

func TestWebUnpark_ZeroCommitWithAGoneAgentReopensAndLogsTheAnswer(t *testing.T) {
	e := startAnswerEnv(t, nil)
	state := parkEnvZeroCommit(t, e)
	if err := e.fake.Stop(mustFind(t, e.fake)); err != nil {
		t.Fatal(err)
	}

	out := unparkResult(t, webUnpark(t, e.h, server.RepairRequest{State: state}))
	if out.Status != "open" || out.Message != repair.MessageReopened {
		t.Errorf("result = %+v; want open, %q", out, repair.MessageReopened)
	}
	if tk, err := schema.ParseTicket(e.ticket); err != nil || tk.Status != schema.StatusOpen {
		t.Errorf("ticket = %+v, %v; want open", tk, err)
	}
	got := answerEvents(t, e.ticket)
	if len(got) != 1 || got[0].Choice != "unpark" || got[0].Source != "web" || got[0].Base != "" {
		t.Errorf("answers = %+v; want one web park.answer{choice: unpark}", got)
	}
}

func TestWebUnpark_ZeroCommitWithALiveAgentPromptsAndKeepsThePark(t *testing.T) {
	e := startAnswerEnv(t, nil)
	state := parkEnvZeroCommit(t, e)

	out := unparkResult(t, webUnpark(t, e.h, server.RepairRequest{State: state}))
	if out.Status != repair.UnparkPrompted || out.Message != repair.MessagePrompted {
		t.Errorf("result = %+v; want prompted", out)
	}
	want := "A person reviewed your finish with no commits and wants you to continue. " +
		"Finish the work, or report `--iteration-status finished --commitless true`."
	if got := e.fake.Prompts(answerLabel); len(got) != 1 || got[0] != want {
		t.Errorf("prompts = %q; want [%q]", got, want)
	}
	if tk, err := schema.ParseTicket(e.ticket); err != nil || tk.Status != schema.StatusNeedsAnswer {
		t.Errorf("ticket = %+v, %v; want still needs-answer", tk, err)
	}
}

func TestWebUnpark_ZeroCommitWithALateFinishIsAdopted(t *testing.T) {
	p := parkZeroCommit(t, false)
	state := stateOf(t, p.ticketPath)

	// The agent reports a commitless finish after the park. No rescan ping, so
	// the unpark is the one that adopts it.
	if err := schema.UpdateTicket(p.ticketPath, p.finishedLate(true)); err != nil {
		t.Fatal(err)
	}
	out := unparkResult(t, webUnpark(t, p.h, server.RepairRequest{State: state}))
	if out.Status != repair.UnparkAdopted || out.Message != repair.MessageAdopted {
		t.Errorf("result = %+v; want adopted as done", out)
	}
	if tk, err := schema.ParseTicket(p.ticketPath); err != nil || tk.Status != schema.StatusDone {
		t.Errorf("ticket = %+v, %v; want done", tk, err)
	}
}

func TestWebUnpark_AmbiguousBaseMustBeACompetingBlocker(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "03", "third", servertest.TicketOpts{BlockedBy: []string{"01", "02"}})
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	registerLaunch(h)
	if _, err := h.Server.ClaimTicket("proj:epic-a/03", "claude"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store, "proj", "epic-a", "issues", "03-third.md")
	state := stateOf(t, path)
	req := server.RepairRequest{Address: "proj:epic-a/03", State: state, Base: "99"}

	res, err := h.Server.UnparkAsWeb(req)
	if err != nil || !res.Refused || res.Reason != server.ReasonBadBase {
		t.Fatalf("unpark with a foreign base = %+v, %v; want refusal %s", res, err, server.ReasonBadBase)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusNeedsAnswer || tk.Base != "" {
		t.Fatalf("ticket = %+v, %v; want untouched", tk, err)
	}

	req.Base = "02"
	if res, err := h.Server.UnparkAsWeb(req); err != nil || res.Refused {
		t.Fatalf("unpark with a competing base = %+v, %v", res, err)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusOpen || tk.Base != "02" {
		t.Errorf("ticket = %+v, %v; want open with base 02", tk, err)
	}
	if got := answerEvents(t, path); len(got) != 1 || got[0].Choice != "unpark" || got[0].Base != "02" {
		t.Errorf("answers = %+v; want park.answer{choice: unpark, base: 02}", got)
	}
}

func TestDetail_AmbiguousBaseListsTheCompetingBlockers(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "03", "third", servertest.TicketOpts{BlockedBy: []string{"01", "02"}})
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	registerLaunch(h)
	if _, err := h.Server.ClaimTicket("proj:epic-a/03", "claude"); err != nil {
		t.Fatal(err)
	}
	d, err := h.Client.Detail(context.Background(), "proj:epic-a/03")
	if err != nil || len(d.Bases) != 2 || !slices.Contains(d.Bases, "01") || !slices.Contains(d.Bases, "02") {
		t.Errorf("bases = %v, %v; want 01 and 02", d.Bases, err)
	}
}

func TestUnpark_CLIWithoutATokenStillWorks(t *testing.T) {
	e := startAnswerEnv(t, nil)
	parkEnvZeroCommit(t, e)
	if err := e.fake.Stop(mustFind(t, e.fake)); err != nil {
		t.Fatal(err)
	}

	res, err := e.h.Client.Repair(context.Background(), "unpark", server.RepairRequest{Address: answerAddr})
	if err != nil || res.Refused {
		t.Fatalf("unpark = %+v, %v; want reopened", res, err)
	}
	if tk, err := schema.ParseTicket(e.ticket); err != nil || tk.Status != schema.StatusOpen {
		t.Errorf("ticket = %+v, %v; want open", tk, err)
	}
	if got := answerEvents(t, e.ticket); len(got) != 0 {
		t.Errorf("the CLI path logged %+v", got)
	}
}
