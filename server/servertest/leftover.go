package servertest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// LeavePtyRun stages what a server that died mid-iteration leaves behind for
// proj:epic-a/01 (as WriteTicket and SetProjectRepo made it): a claimed ticket,
// its worktree with the agent's commit, and a runs.json entry on the pty runner.
// The next Restart reclaims it.
//
// A live run is a real holder and TUI fake the reclaim reattaches to. A dead
// run is an agent directory whose holder and agent are gone. It returns the
// worktree path and the ticket file.
func (h *Harness) LeavePtyRun(t *testing.T, store, repo string, live bool) (wtPath, ticketPath string) {
	t.Helper()
	epics, err := tickets.Load(filepath.Join(store, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	tk := epics[0].Tickets[0]
	if err := ralphloop.Claim(tk.Path); err != nil {
		t.Fatal(err)
	}
	wt, err := ralphloop.PrepareIteration(ralphloop.DefaultDeps(), ralphloop.OneIteration{
		RepoDir: repo, Epic: "epic-a", ScratchDir: store, Agent: ralphloop.AgentClaude, Ticket: tk,
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, wt.Path, "agent.txt", "work")
	testutil.CommitAll(t, wt.Path, "agent work")

	label, _, _ := ralphloop.IterationIdentity("epic-a", tk.Identifier, "")
	session := agentrunner.Session{Label: label, ID: label}
	if live {
		// The embedded Pty, so the staged launch is not counted by Starts.
		session, err = h.cfg.RunnerFor("pty", "proj").(*ptyRunner).Pty.Start(agentrunner.StartOptions{
			Label: label, Epic: "epic-a", Cwd: wt.Path, Kind: agentrunner.Kind("claude"),
		})
		if err != nil {
			t.Fatal(err)
		}
	} else {
		dir := filepath.Join(nativerunner.AgentsRoot(h.StateDir, "proj"), label)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		meta := nativerunner.Meta{
			Runner: "pty", PID: deadPID, PIDStart: "gone", HolderPID: deadPID, HolderPIDStart: "gone",
			SessionID: "dead-session", Label: label, Epic: "epic-a", Cwd: wt.Path,
		}
		if err := nativerunner.WriteMeta(dir, meta); err != nil {
			t.Fatal(err)
		}
	}
	handle, err := json.Marshal([]map[string]any{{
		"address": "proj:epic-a/01", "agent": "claude", "runner": "pty", "session": session, "root": "proj:epic-a",
		"repo": repo, "base": wt.Base(), "ticket_path": tk.Path,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.StateDir, "runs.json"), handle, 0o600); err != nil {
		t.Fatal(err)
	}
	return wt.Path, tk.Path
}

// deadPID is a pid no process table lists.
const deadPID = 1<<31 - 2
