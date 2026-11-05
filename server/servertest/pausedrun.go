package servertest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// AnswerAddr is the address of the ticket the answer and paused-run helpers use.
const AnswerAddr = "proj:epic-a/01"

// OptionsBlock is a question block with two options, a and b.
const OptionsBlock = "type: options\noptions:\n  - {id: a, label: A}\n  - {id: b, label: B}\n"

// EpicA01 is the ticket WriteTicket(t, store, "proj", "epic-a", "01", ...) makes.
var EpicA01 = Target{Project: "proj", Epic: "epic-a", Ticket: "01"}

// RunGitIn runs git with args in repo.
func RunGitIn(t *testing.T, repo string, args ...string) {
	t.Helper()
	f := SchedFixture{Repo: repo}
	f.Git(t, args...)
}

// LeaveIteration stages what a server that died mid-iteration leaves behind: a
// claimed ticket, its worktree with the agent's commit, and the persisted
// handle. It returns the worktree path and the ticket file.
func LeaveIteration(t *testing.T, h *Harness, store, repo, runner string) (string, string) {
	t.Helper()
	return h.LeaveHerdrRun(t, store, repo, EpicA01, runner, func(wtPath string) {
		testutil.WriteFile(t, wtPath, "agent.txt", "work")
		testutil.CommitAll(t, wtPath, "agent work")
	})
}

// PausedIteration leaves ticket 01 paused as kind with an iteration branch,
// committed to when commit is set, and returns the ticket's path.
func PausedIteration(t *testing.T, kind string, commit bool) (*Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", repo)
	h := StartWithStore(t, store)
	wt, ticketPath := LeaveIteration(t, h, store, repo, "herdr")
	_ = os.Remove(filepath.Join(h.StateDir, "runs.json"))
	if !commit {
		RunGitIn(t, wt, "reset", "-q", "--hard", "HEAD~1")
	}
	pause := "status: paused\npause_reason: " + kind
	testutil.WriteFile(t, filepath.Dir(ticketPath), filepath.Base(ticketPath), strings.Replace(ReadFile(t, ticketPath), "status: claimed", pause, 1))
	return h, ticketPath
}

// AsHeadlessRun rewrites the staged handle as a headless run on the fake runner.
func AsHeadlessRun(t *testing.T, h *Harness) {
	t.Helper()
	StampRun(t, h, "fake", agentrunner.Session{Label: "epic-a-iter-01", ID: "p1"})
}

// StampRun rewrites the staged handle's runner and session.
func StampRun(t *testing.T, h *Harness, runner string, session agentrunner.Session) {
	t.Helper()
	if runner == "" {
		t.Fatal("StampRun needs a runner")
	}
	path := filepath.Join(h.StateDir, "runs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs []map[string]any
	if err := json.Unmarshal(data, &runs); err != nil {
		t.Fatal(err)
	}
	runs[0]["runner"] = runner
	runs[0]["session"] = session
	if _, ok := runs[0]["agent"]; !ok {
		runs[0]["agent"] = "claude" // a real launch always records it
	}
	delete(runs[0], "pane")
	if data, err = json.Marshal(runs); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// MarkWaiting stamps the staged handle as waiting for an answer, as a server
// that was stopped while the agent waited would have saved it.
func MarkWaiting(t *testing.T, h *Harness, since time.Time) {
	t.Helper()
	path := filepath.Join(h.StateDir, "runs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs []map[string]any
	if err := json.Unmarshal(data, &runs); err != nil {
		t.Fatal(err)
	}
	runs[0]["waiting_since"] = since
	if data, err = json.Marshal(runs); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// StartWaitingRestartFake restarts a server over a run that was waiting for an
// answer, and also returns the runner hosting the waiting agent, for tests
// that read its prompts.
func StartWaitingRestartFake(t *testing.T, hardLimit float64, tweak ...func(*server.Config)) (*Harness, string, *HeadlessFake) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", repo)
	fake := NewHeadlessFake(nativerunner.VerdictLive)
	h := StartWithStore(t, store, WithRunner(fake), func(c *server.Config) {
		c.BudgetHardLimit = hardLimit
		c.BudgetKillGrace = time.Millisecond
		c.BudgetPollInterval = time.Hour
		for _, f := range tweak {
			f(c)
		}
	})
	wtPath, ticketPath := LeaveIteration(t, h, store, repo, "herdr")
	// The agent is still alive on the runner while the server is down.
	if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01", Cwd: wtPath}); err != nil {
		t.Fatal(err)
	}
	AsHeadlessRun(t, h)
	MarkWaiting(t, h, time.Now().Add(-time.Hour))
	h.Restart(t)
	return h, ticketPath, fake
}

// PauseWithQuestion turns the waiting run's ticket into a self-reported pause
// with one open question, and returns the state token a client would hold.
func PauseWithQuestion(t *testing.T, ticketPath, block string) string {
	t.Helper()
	err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
		tk.Status = schema.StatusLegacyAnswer
		tk.PauseKind = schema.PauseKindSelfReported
	})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := tickets.AppendLog(ticketPath, tickets.LogEvent{
		Type: tickets.EventPauseQuestion, By: "agent", Summary: "which?", Handoff: "h", Block: block,
	})
	if err != nil {
		t.Fatal(err)
	}
	return AnswerToken(ev.Seq, schema.PauseClassAnswer)
}

// AnswerToken builds the state token for the question at seq.
func AnswerToken(seq int, class schema.PauseClass) string {
	return strings.Join([]string{string(rune('0' + seq)), string(class)}, ":")
}
