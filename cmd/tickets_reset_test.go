package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
)

type resetFixture struct {
	in         resetInput
	deps       ralphloop.Deps
	ticketPath string
	epicPath   string
	branchOK   bool
	agentAlive bool
	wtExists   bool
	tabLive    bool
	calls      []string
}

func newResetFixture(t *testing.T, content string) *resetFixture {
	t.Helper()
	epicPath, ticketPath := unparkFixture(t, content)
	f := &resetFixture{epicPath: epicPath, ticketPath: ticketPath, branchOK: true}
	f.deps = ralphloop.Deps{
		WorktreeDir: func(string) (string, error) { return t.TempDir(), nil },
		RevParse: func(_, ref string) (string, error) {
			if strings.HasPrefix(ref, "ralph-loop/attic/") || !f.branchOK {
				return "", errors.New("unknown revision")
			}
			return "tip123", nil
		},
		WorktreeExists: func(string) (bool, error) { return f.wtExists, nil },
		RemoveWorktree: func(_, path string, _ bool) error {
			f.calls = append(f.calls, "remove-worktree "+filepath.Base(path))
			return nil
		},
		FindWorkspace: func(string) (string, error) { return "ws1", nil },
		TabList: func(string) ([]herdr.Tab, error) {
			if !f.tabLive {
				return nil, nil
			}
			return []herdr.Tab{{TabID: "tab1", Label: "widget-epic-iter-01"}}, nil
		},
		TabClose: func(id string) error { f.calls = append(f.calls, "close-tab "+id); return nil },
		RenameBranch: func(_, from, to string) error {
			f.calls = append(f.calls, "rename "+from+" "+to)
			return nil
		},
		DeleteBranch: func(_, b string) error { f.calls = append(f.calls, "delete "+b); return nil },
		AgentGet: func(string) (herdr.Agent, error) {
			if f.agentAlive {
				return herdr.Agent{PaneID: "p1"}, nil
			}
			return herdr.Agent{}, errors.New("not found")
		},
	}
	f.in = resetInput{
		EpicPath: epicPath,
		ID:       "01",
		Reason:   "agent went in circles",
		JSON:     true,
		Cwd:      t.TempDir(),
		Now:      time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC),
		Subjects: func(_, _, _ string) ([]string, error) { return []string{"second", "first"}, nil },
	}
	return f
}

func (f *resetFixture) run() (string, error) {
	var out, errOut bytes.Buffer
	err := runTicketsReset(f.in, f.deps, &out, &errOut)
	return out.String(), err
}

func (f *resetFixture) refusalReason(t *testing.T) string {
	t.Helper()
	out, err := f.run()
	assertExit1(t, err)
	var env RefusalEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil || !env.Refused {
		t.Fatalf("stdout %q: %v", out, jerr)
	}
	return env.Reason
}

func (f *resetFixture) ticketText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(f.ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunTicketsReset_StatusRules(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"claimed", "needs-repair"} {
		t.Run("accepts "+status, func(t *testing.T) {
			t.Parallel()
			f := newResetFixture(t, ticketWith(status, ""))
			if _, err := f.run(); err != nil {
				t.Fatalf("err = %v", err)
			}
			if !strings.Contains(f.ticketText(t), "status: open") {
				t.Errorf("ticket not reopened:\n%s", f.ticketText(t))
			}
		})
	}
	for _, status := range []string{"draft", "open"} {
		t.Run("refuses "+status, func(t *testing.T) {
			t.Parallel()
			f := newResetFixture(t, ticketWith(status, ""))
			if got := f.refusalReason(t); got != ReasonStatusRefused {
				t.Errorf("reason = %q", got)
			}
		})
	}
}

func TestRunTicketsReset_DoneNeedsForce(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("done", ""))
	if got := f.refusalReason(t); got != ReasonStatusRefused {
		t.Errorf("reason = %q", got)
	}
	f.in.Force = true
	if _, err := f.run(); err != nil {
		t.Fatalf("forced err = %v", err)
	}
}

func (f *resetFixture) addTicket(t *testing.T, file, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.epicPath, "issues", file), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRunTicketsReset_ClaimedWithOpenBlockerAccepted(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", "blocked_by: [\"02\"]\n"))
	f.addTicket(t, "02-b.md", "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n")
	if _, err := f.run(); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(f.ticketText(t), "status: open") {
		t.Errorf("ticket not reopened:\n%s", f.ticketText(t))
	}
}

func TestRunTicketsReset_DoneBlockingDependentsAcceptedWithForce(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("done", ""))
	f.addTicket(t, "02-b.md", "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# B\n")
	if got := f.refusalReason(t); got != ReasonStatusRefused {
		t.Errorf("reason = %q", got)
	}
	f.in.Force = true
	if _, err := f.run(); err != nil {
		t.Fatalf("forced err = %v", err)
	}
}

func TestRunTicketsReset_NeedsAnswerAccepted(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, parkedTicket)
	if _, err := f.run(); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestRunTicketsReset_MissingReason(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	f.in.Reason = "  "
	if got := f.refusalReason(t); got != ReasonReasonRequired {
		t.Errorf("reason = %q", got)
	}
}

func TestRunTicketsReset_ForkChildrenRefusedEvenForced(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	child := "---\nid: \"02\"\nstatus: open\ntype: task\nparent: \"01\"\n---\n# B\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(f.ticketPath), "02-b.md"), []byte(child), 0644); err != nil {
		t.Fatal(err)
	}
	f.in.Force = true
	out, err := f.run()
	assertExit1(t, err)
	if !strings.Contains(out, ReasonForkChildren) || !strings.Contains(out, "reset 02 instead") {
		t.Errorf("stdout = %s", out)
	}
}

func TestRunTicketsReset_LiveAgentRefused(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	f.agentAlive = true
	if got := f.refusalReason(t); got != ReasonLiveAgentOnTab {
		t.Errorf("reason = %q", got)
	}
	if !strings.Contains(f.ticketText(t), "status: claimed") {
		t.Error("refused reset must not touch the ticket")
	}
}

func TestRunTicketsReset_CommentIsFramedAsUnverified(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	out, err := f.run()
	if err != nil {
		t.Fatal(err)
	}
	var res resetResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res.AtticRef == nil || *res.AtticRef != "ralph-loop/attic/widget-epic/01-1" {
		t.Errorf("attic ref = %v", res.AtticRef)
	}
	text := f.ticketText(t)
	for _, want := range []string{"## Comments", "agent went in circles", "unverified", "ralph-loop/attic/widget-epic/01-1", "tip123", "Commits: 2", "- second", "- first"} {
		if !strings.Contains(text, want) {
			t.Errorf("ticket missing %q:\n%s", want, text)
		}
	}
}

func TestRunTicketsReset_MissingBranchIsNormal(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	f.branchOK = false
	out, err := f.run()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"attic_ref":null`) {
		t.Errorf("stdout = %s", out)
	}
	if !strings.Contains(f.ticketText(t), "No iteration branch existed") {
		t.Errorf("ticket:\n%s", f.ticketText(t))
	}
}

func TestRunTicketsReset_WritesEvent(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	if _, err := f.run(); err != nil {
		t.Fatal(err)
	}
	events, ok, err := ralphloop.ReadEvents(filepath.Dir(f.epicPath), "widget-epic")
	if err != nil || !ok || len(events) != 1 {
		t.Fatalf("events = %v ok=%v err=%v", events, ok, err)
	}
	ev := events[0]
	if ev.Type != ralphloop.EventTicketReset || ev.Ticket != "01" || ev.AtticRef != "ralph-loop/attic/widget-epic/01-1" || ev.Reason != "agent went in circles" {
		t.Errorf("event = %+v", ev)
	}
}

func TestRunTicketsReset_ClearsWorktreeTabAndAtticsBranch(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	f.wtExists, f.tabLive = true, true
	if _, err := f.run(); err != nil {
		t.Fatal(err)
	}
	want := []string{"remove-worktree widget-epic-item-01", "close-tab tab1", "rename ralph-loop/widget-epic-item-01 ralph-loop/attic/widget-epic/01-1"}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %q", f.calls)
	}
}

func TestRunTicketsReset_DeleteBranch(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	f.in.DeleteBranch = true
	out, err := f.run()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls, "|"); got != "delete ralph-loop/widget-epic-item-01" {
		t.Errorf("calls = %q", got)
	}
	if !strings.Contains(out, `"attic_ref":null`) || !strings.Contains(f.ticketText(t), "--delete-branch") {
		t.Errorf("stdout = %s\nticket:\n%s", out, f.ticketText(t))
	}
}

func TestRunTicketsReset_MissingBranchTouchesNoGit(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	f.branchOK = false
	if _, err := f.run(); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("calls = %v", f.calls)
	}
}
