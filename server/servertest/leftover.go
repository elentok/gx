package servertest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// Target names the ticket a leftover iteration is staged for, as WriteTicket
// and SetProjectRepo made it.
type Target struct {
	Project, Epic, Ticket string
}

// Address is the ticket's address, e.g. "proj:epic-a/01".
func (tg Target) Address() string { return tg.Project + ":" + tg.Epic + "/" + tg.Ticket }

// Run is a claimed ticket and its iteration worktree.
type Run struct {
	Ticket   tickets.Ticket
	Worktree ralphloop.IterationWorktree
}

// LeaveRun stages what a server that died mid-iteration leaves behind: a
// claimed ticket, its worktree, and a runs.json entry. work is what the agent
// did in its worktree (nil leaves an empty claim). handle returns the entry's
// runner-specific fields (runner, session, pane, ...); LeaveRun adds the rest.
// It returns the worktree path and the ticket file.
func (h *Harness) LeaveRun(t *testing.T, store, repo string, tg Target, work func(wtPath string), handle func(Run) map[string]any) (wtPath, ticketPath string) {
	t.Helper()
	epics, err := tickets.Load(filepath.Join(store, tg.Project))
	if err != nil {
		t.Fatal(err)
	}
	var tk tickets.Ticket
	found := false
	for _, e := range epics {
		if e.Name != tg.Epic {
			continue
		}
		for _, c := range e.Tickets {
			if c.Identifier == tg.Ticket {
				tk, found = c, true
			}
		}
	}
	if !found {
		t.Fatalf("no ticket %s in %s", tg.Ticket, store)
	}
	if err := ralphloop.Claim(tk.Path); err != nil {
		t.Fatal(err)
	}
	wt, err := ralphloop.PrepareIteration(ralphloop.DefaultDeps(), ralphloop.OneIteration{
		RepoDir: repo, Epic: tg.Epic, ScratchDir: store, Agent: ralphloop.AgentClaude, Ticket: tk,
	})
	if err != nil {
		t.Fatal(err)
	}
	if work != nil {
		work(wt.Path)
	}

	entry := handle(Run{Ticket: tk, Worktree: wt})
	for k, v := range map[string]any{
		"address": tg.Address(), "agent": "claude", "root": tg.Project + ":" + tg.Epic,
		"repo": repo, "base": wt.Base(), "ticket_path": tk.Path,
	} {
		entry[k] = v
	}
	data, err := json.Marshal([]map[string]any{entry})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.StateDir, "runs.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return wt.Path, tk.Path
}

// LeaveHerdrRun is LeaveRun for a run on runner (a herdr-style runner) in pane
// p1 of tab t1 and workspace w1.
func (h *Harness) LeaveHerdrRun(t *testing.T, store, repo string, tg Target, runner string, work func(wtPath string)) (wtPath, ticketPath string) {
	t.Helper()
	return h.LeaveRun(t, store, repo, tg, work, func(Run) map[string]any {
		return map[string]any{"runner": runner, "pane": "p1", "tab": "t1", "workspace": "w1"}
	})
}

// LeavePtyRun stages a leftover iteration of tg on the pty runner. The next
// Restart reclaims it.
//
// work is what the agent did in its worktree; nil leaves an empty claim. A live
// run is a real holder and TUI fake the reclaim reattaches to. A dead run is an
// agent directory whose holder and agent are gone.
func (h *Harness) LeavePtyRun(t *testing.T, store, repo string, tg Target, live bool, work func(wtPath string)) (wtPath, ticketPath string) {
	t.Helper()
	return h.LeaveRun(t, store, repo, tg, work, func(r Run) map[string]any {
		wt := r.Worktree
		label, _, _ := ralphloop.IterationIdentity(tg.Epic, r.Ticket.Identifier, "")
		session := agentrunner.Session{Label: label, ID: label, SessionID: "dead-session"}
		if live {
			var err error
			// The embedded Pty, so the staged launch is not counted by Starts.
			session, err = h.cfg.RunnerFor("pty", tg.Project).(*ptyRunner).Pty.Start(agentrunner.StartOptions{
				Label: label, Epic: tg.Epic, Cwd: wt.Path, Kind: agentrunner.Kind("claude"),
			})
			if err != nil {
				t.Fatal(err)
			}
		} else {
			dir := filepath.Join(nativerunner.AgentsRoot(h.StateDir, tg.Project), label)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			meta := nativerunner.Meta{
				Runner: "pty", PID: deadPID, PIDStart: "gone", HolderPID: deadPID, HolderPIDStart: "gone",
				SessionID: "dead-session", Label: label, Epic: tg.Epic, Cwd: wt.Path,
			}
			if err := nativerunner.WriteMeta(dir, meta); err != nil {
				t.Fatal(err)
			}
		}
		return map[string]any{"runner": "pty", "session": session}
	})
}

// deadPID is a pid no process table lists.
const deadPID = 1<<31 - 2
