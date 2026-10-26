package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// pauseEnvZeroCommit pauses ticket 01 of the answer env as zero-commit and returns
// the state token of its question.
func pauseEnvZeroCommit(t *testing.T, e answerEnv) string {
	t.Helper()
	if err := e.h.Server.PauseAs(answerAddr, events.ZeroCommit, "no commits"); err != nil {
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
	return fmt.Sprintf("%d:%s", open.Seq, tk.PausedClass())
}

func webResume(t *testing.T, h *servertest.Harness, req server.RepairRequest) server.RepairResult {
	t.Helper()
	req.Address = answerAddr
	res, err := h.Server.ResumeAsWeb(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func resumeResult(t *testing.T, res server.RepairResult) repair.ResumeResult {
	t.Helper()
	if res.Refused {
		t.Fatalf("resume refused: %+v", res)
	}
	var out repair.ResumeResult
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
		if ev.Type == tickets.EventPauseAnswer {
			out = append(out, ev)
		}
	}
	return out
}

func TestWebResume_MissingOrStaleTokenRefusesBeforeAnyChange(t *testing.T) {
	e := startAnswerEnv(t, nil)
	pauseEnvZeroCommit(t, e)

	for name, state := range map[string]string{"missing": "", "stale": "99:paused"} {
		res := webResume(t, e.h, server.RepairRequest{State: state})
		if !res.Refused || res.Reason != server.ReasonStale {
			t.Fatalf("%s: resume = %+v; want refusal %s", name, res, server.ReasonStale)
		}
	}
	if tk, err := schema.ParseTicket(e.ticket); err != nil || tk.PausedClass() != schema.PauseClassAnswer {
		t.Errorf("ticket = %+v, %v; want still asking", tk, err)
	}
	if got := answerEvents(t, e.ticket); len(got) != 0 {
		t.Errorf("refusal logged %+v", got)
	}
	if got := e.fake.Prompts(answerLabel); len(got) != 0 {
		t.Errorf("refusal prompted the agent: %q", got)
	}
}

func TestWebResume_ZeroCommitWithAGoneAgentReopensAndLogsTheAnswer(t *testing.T) {
	e := startAnswerEnv(t, nil)
	state := pauseEnvZeroCommit(t, e)
	if err := e.fake.Stop(mustFind(t, e.fake)); err != nil {
		t.Fatal(err)
	}

	out := resumeResult(t, webResume(t, e.h, server.RepairRequest{State: state}))
	if out.Status != "open" || out.Message != repair.MessageReopened {
		t.Errorf("result = %+v; want open, %q", out, repair.MessageReopened)
	}
	if tk, err := schema.ParseTicket(e.ticket); err != nil || tk.Status != schema.StatusOpen {
		t.Errorf("ticket = %+v, %v; want open", tk, err)
	}
	got := answerEvents(t, e.ticket)
	if len(got) != 1 || got[0].Choice != "unpark" || got[0].Source != "web" || got[0].How != tickets.HowResume || got[0].Base != "" {
		t.Errorf("answers = %+v; want one web pause.answer{choice: unpark, how: resume}", got)
	}
}

func TestWebResume_SelfReportedWithAGoneAgentEndsTheRoundAsResume(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	sess, ok, err := fake.Find("epic-a-iter-01")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}
	if err := fake.Stop(sess); err != nil {
		t.Fatal(err)
	}

	resumeResult(t, webResume(t, h, server.RepairRequest{State: state}))
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	rounds := tickets.Rounds(evs)
	if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowResume || rounds[0].End.Source != tickets.SourceWeb {
		t.Errorf("rounds = %+v; want one ended by web resume", rounds)
	}
}

func TestWebResume_ZeroCommitWithALiveAgentPromptsAndKeepsThePause(t *testing.T) {
	e := startAnswerEnv(t, nil)
	state := pauseEnvZeroCommit(t, e)

	out := resumeResult(t, webResume(t, e.h, server.RepairRequest{State: state}))
	if out.Status != repair.ResumePrompted || out.Message != repair.MessagePrompted {
		t.Errorf("result = %+v; want prompted", out)
	}
	want := "A person reviewed your finish with no commits and wants you to continue. " +
		"Finish the work, or report `--iteration-status finished --commitless true`."
	if got := e.fake.Prompts(answerLabel); len(got) != 1 || got[0] != want {
		t.Errorf("prompts = %q; want [%q]", got, want)
	}
	if tk, err := schema.ParseTicket(e.ticket); err != nil || tk.PausedClass() != schema.PauseClassAnswer {
		t.Errorf("ticket = %+v, %v; want still asking", tk, err)
	}
}

func TestWebResume_ZeroCommitPromptKeepsTheRoundOpenAndIsLogged(t *testing.T) {
	e := startAnswerEnv(t, nil, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	state := pauseEnvZeroCommit(t, e)
	hs, err := e.h.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake = %+v, %v", hs, err)
	}

	resumeResult(t, webResume(t, e.h, server.RepairRequest{State: state}))

	evs, err := tickets.ReadLog(e.ticket)
	if err != nil {
		t.Fatal(err)
	}
	var prompts []tickets.LogEvent
	for _, ev := range evs {
		if ev.Type == tickets.EventPausePrompted {
			prompts = append(prompts, ev)
		}
	}
	open := tickets.OpenRound(evs)
	if len(prompts) != 1 || open == nil || prompts[0].Question != open.Seq || prompts[0].Source != tickets.SourceWeb {
		t.Fatalf("prompts = %+v, open = %+v; want one web pause.prompted on the open round", prompts, open)
	}
	if got := stateOf(t, e.ticket); got != state {
		t.Errorf("state = %q, want unchanged %q", got, state)
	}

	code, body := webDo(t, e.h, http.MethodGet, hs.WebURL+"/v1/tickets/detail?address="+answerAddr)
	if code != http.StatusOK {
		t.Fatalf("detail = %d %s", code, body)
	}
	var d server.TicketDetail
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	want := "Prompt sent to the running agent at " + prompts[0].TS.Local().Format("15:04")
	if d.Latest == nil || d.Latest.Outcome != server.RoundOpen || d.Latest.Prompt != want {
		t.Errorf("latest = %+v; want an open round with prompt %q", d.Latest, want)
	}

	runLog, err := os.ReadFile(filepath.Join(e.h.TicketStore, "proj", "epic-a", "run-log.jsonl"))
	if err != nil || !strings.Contains(string(runLog), `"type":"`+string(events.PausePrompted)+`"`) {
		t.Errorf("run log = %q, %v; want a pause-prompted event", runLog, err)
	}
	srvLog, err := os.ReadFile(server.LogPath(e.h.StateDir))
	if err != nil || !strings.Contains(string(srvLog), "prompted the live agent") {
		t.Errorf("server.log = %q, %v; want the prompt INFO line", srvLog, err)
	}
}

func TestWebResume_ZeroCommitWithALateFinishIsAdopted(t *testing.T) {
	p := pauseZeroCommit(t, false)
	state := stateOf(t, p.ticketPath)

	// The agent reports a commitless finish after the pause. No rescan ping, so
	// the resume is the one that adopts it.
	if err := schema.UpdateTicket(p.ticketPath, p.finishedLate(true)); err != nil {
		t.Fatal(err)
	}
	out := resumeResult(t, webResume(t, p.h, server.RepairRequest{State: state}))
	if out.Status != repair.ResumeAdopted || out.Message != repair.MessageAdopted {
		t.Errorf("result = %+v; want adopted as done", out)
	}
	if tk, err := schema.ParseTicket(p.ticketPath); err != nil || tk.Status != schema.StatusDone {
		t.Errorf("ticket = %+v, %v; want done", tk, err)
	}
}

func TestWebResume_AmbiguousBaseMustBeACompetingBlocker(t *testing.T) {
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

	res, err := h.Server.ResumeAsWeb(req)
	if err != nil || !res.Refused || res.Reason != server.ReasonBadBase {
		t.Fatalf("resume with a foreign base = %+v, %v; want refusal %s", res, err, server.ReasonBadBase)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.PausedClass() != schema.PauseClassAnswer || tk.Base != "" {
		t.Fatalf("ticket = %+v, %v; want untouched", tk, err)
	}

	req.Base = "02"
	if res, err := h.Server.ResumeAsWeb(req); err != nil || res.Refused {
		t.Fatalf("resume with a competing base = %+v, %v", res, err)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusOpen || tk.Base != "02" {
		t.Errorf("ticket = %+v, %v; want open with base 02", tk, err)
	}
	if got := answerEvents(t, path); len(got) != 1 || got[0].Choice != "unpark" || got[0].Base != "02" {
		t.Errorf("answers = %+v; want pause.answer{choice: resume, base: 02}", got)
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

func TestResume_CLIWithoutATokenStillWorks(t *testing.T) {
	e := startAnswerEnv(t, nil)
	pauseEnvZeroCommit(t, e)
	if err := e.fake.Stop(mustFind(t, e.fake)); err != nil {
		t.Fatal(err)
	}

	res, err := e.h.Client.Repair(context.Background(), "resume", server.RepairRequest{Address: answerAddr})
	if err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v; want reopened", res, err)
	}
	if tk, err := schema.ParseTicket(e.ticket); err != nil || tk.Status != schema.StatusOpen {
		t.Errorf("ticket = %+v, %v; want open", tk, err)
	}
	if got := answerEvents(t, e.ticket); len(got) != 0 {
		t.Errorf("the CLI path logged %+v", got)
	}
}

func TestWebResume_AFailedReopenLeavesNoAnswerAndTheNextResumeWorks(t *testing.T) {
	e := startAnswerEnv(t, nil)
	state := pauseEnvZeroCommit(t, e)
	if err := e.fake.Stop(mustFind(t, e.fake)); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Dir(e.ticket)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if !restored {
			restored = true
			if err := os.Chmod(dir, 0o755); err != nil {
				t.Error(err)
			}
		}
	}
	defer restore()

	if res, err := e.h.Server.ResumeAsWeb(server.RepairRequest{Address: answerAddr, State: state}); err == nil && !res.Refused {
		t.Fatalf("resume = %+v; want a failure", res)
	}
	restore()
	if got := answerEvents(t, e.ticket); len(got) != 0 {
		t.Errorf("failed resume logged %+v", got)
	}

	out := resumeResult(t, webResume(t, e.h, server.RepairRequest{State: state}))
	if out.Status != "open" {
		t.Errorf("retry = %+v; want open", out)
	}
	if got := answerEvents(t, e.ticket); len(got) != 1 {
		t.Errorf("answers = %+v; want one", got)
	}
}

func TestWebResume_AnUnreadableLogIsAnErrorNotStale(t *testing.T) {
	e := startAnswerEnv(t, nil)
	state := pauseEnvZeroCommit(t, e)

	logPath := tickets.LogPath(e.ticket)
	if err := os.Chmod(logPath, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(logPath, 0o644)

	res, err := e.h.Server.ResumeAsWeb(server.RepairRequest{Address: answerAddr, State: state})
	if err == nil {
		t.Fatalf("resume = %+v; want an error", res)
	}
	if res.Refused {
		t.Errorf("resume = %+v; a read error must not be a stale refusal", res)
	}
}
