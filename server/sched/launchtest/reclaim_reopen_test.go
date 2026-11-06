package launchtest_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets/schema"
)

// restartDead leaves an iteration whose agent is gone after a restart: the pty
// agent's holder and process are dead, or the headless runner reports it died.
// work is what the agent did in its worktree. It returns the store, repo,
// worktree and ticket.
func restartDead(t *testing.T, headless bool, work func(t *testing.T, wtPath string)) (string, string, string, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	var h *servertest.Harness
	var wtPath, ticketPath string
	if headless {
		fake := servertest.NewHeadlessFake(nativerunner.VerdictDied)
		h = servertest.StartWithStore(t, store, servertest.WithRunner(fake))
		wtPath, ticketPath = leaveIterationWith(t, h, store, repo, runnerOf(headless), func(p string) { work(t, p) })
		stampRun(t, h, runnerOf(headless), deadSession(headless))
	} else {
		h = servertest.StartWithStore(t, store)
		wtPath, ticketPath = h.LeavePtyRun(t, store, repo, epicA01, false, func(p string) { work(t, p) })
	}
	h.Restart(t)
	return store, repo, wtPath, ticketPath
}

// deadSession is the session of the run restartDead leaves: the headless fake's
// stamped one, or the dead pty agent's own.
func deadSession(headless bool) agentrunner.Session {
	if headless {
		return agentrunner.Session{Label: "epic-a-iter-01", ID: "p1", SessionID: "sess-1"}
	}
	return agentrunner.Session{Label: "epic-a-iter-01", ID: "epic-a-iter-01", SessionID: "dead-session"}
}

// herdrDeadSession is the session the herdr-subject reclaim tests stamp.
var herdrDeadSession = agentrunner.Session{Label: "epic-a-iter-01", ID: "p1", SessionID: "sess-1"}

func runnerOf(headless bool) string {
	if headless {
		return "fake"
	}
	return "pty"
}

func noWork(*testing.T, string) {}

func editAgentWork(t *testing.T, wtPath string) {
	testutil.WriteFile(t, wtPath, "agent.txt", "work")
}

// assertEmptyClaimReopened checks the reclaim gave the ticket back: open, a
// claim-reopened event, and the worktree and branch removed.
func assertEmptyClaimReopened(t *testing.T, store, repo, wtPath, ticketPath string) {
	t.Helper()
	waitStatus(t, ticketPath, schema.StatusOpen)
	// The reopen writes the ticket's status before its run-log event.
	waitFor(t, func() bool {
		logged, _, _ := runlog.ReadEvents(filepath.Join(store, "proj"), "epic-a")
		return slices.ContainsFunc(logged, func(ev runlog.Event) bool { return ev.Type == string(events.ClaimReopened) })
	})
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree still there: %v", err)
	}
	if _, err := git.RevParse(repo, "ralph-loop/epic-a-item-01"); err == nil {
		t.Error("iteration branch still there")
	}
}

func TestReclaim_DeadEmptyClaimReopens(t *testing.T) {
	t.Parallel()
	runnerShapes(t, func(t *testing.T, headless bool) {
		store, repo, wtPath, ticketPath := restartDead(t, headless, noWork)
		assertEmptyClaimReopened(t, store, repo, wtPath, ticketPath)
	})
}

// herdr answers not-found for the agent, which reclaim reads as gone.
func TestReclaim_HerdrDeadEmptyClaimReopens(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithHerdr(servertest.HerdrIsTheSubject))
	wtPath, ticketPath := leaveIterationWith(t, h, store, repo, "herdr", func(string) {})
	stampRun(t, h, "herdr", herdrDeadSession)
	h.Restart(t)
	assertEmptyClaimReopened(t, store, repo, wtPath, ticketPath)
}

func TestReclaim_HerdrLookupErrorNeverReopens(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithHerdr(servertest.HerdrIsTheSubject))
	h.Herdr.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return nil, herdrfake.Identities{}, errors.New(`{"error":{"code":"internal","message":"herdr hiccup"}}`)
	})
	wtPath, ticketPath := leaveIterationWith(t, h, store, repo, "herdr", func(string) {})
	stampRun(t, h, "herdr", herdrDeadSession)
	h.Restart(t)

	waitStatus(t, ticketPath, schema.StatusLegacyRepair)
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree gone: %v", err)
	}
	if _, err := git.RevParse(repo, "ralph-loop/epic-a-item-01"); err != nil {
		t.Errorf("iteration branch gone: %v", err)
	}
	logged, _, err := runlog.ReadEvents(filepath.Join(store, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(logged, func(ev runlog.Event) bool { return ev.Type == string(events.ClaimReopened) }) {
		t.Errorf("run log has claim-reopened: %v", logged)
	}
}

var assertPauseRecordsRun = servertest.AssertPauseRecordsRun

func TestReclaim_DeadClaimWithWorkPauses(t *testing.T) {
	t.Parallel()
	for name, work := range map[string]func(*testing.T, string){"commit": commitAgentWork, "edit": editAgentWork} {
		t.Run(name, func(t *testing.T) {
			runnerShapes(t, func(t *testing.T, headless bool) {
				store, _, wtPath, ticketPath := restartDead(t, headless, work)
				waitStatus(t, ticketPath, schema.StatusLegacyRepair)
				if _, err := os.Stat(wtPath); err != nil {
					t.Errorf("worktree gone: %v", err)
				}
				assertPauseRecordsRun(t, store, events.Paused, runnerOf(headless), deadSession(headless))
			})
		})
	}
}
