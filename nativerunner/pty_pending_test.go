package nativerunner_test

import (
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

func permissionDialog(header, body string) []string {
	return []string{
		"────────────────────",
		header,
		"  " + body,
		"Do you want to proceed?",
		"❯ 1. Yes",
		"  2. No",
		"Esc to cancel",
	}
}

// blockedOnDialog leaves a fresh pty agent blocked on the given dialog rows.
func blockedOnDialog(t *testing.T, p *nativerunner.Pty, rows []string) agentrunner.Session {
	t.Helper()
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(s) })
	ptyControl(t, s, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: "permission prompt", Dialog: rows})
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	return s
}

func pendingOf(t *testing.T, p *nativerunner.Pty, s agentrunner.Session) *agentrunner.Pending {
	t.Helper()
	st, err := p.Status(s)
	if err != nil {
		t.Fatal(err)
	}
	return st.Pending
}

func TestPty_PendingFilledForApprovableDialog(t *testing.T) {
	p := newPty(t)
	s := blockedOnDialog(t, p, permissionDialog("Bash command", "rg foo"))

	pend := pendingOf(t, p, s)
	if pend == nil || pend.ID == "" || pend.Tool != "Bash command" || pend.Description != "rg foo" {
		t.Fatalf("Pending = %+v; want the Bash command dialog", pend)
	}
}

func TestPty_NoPendingForOtherDialog(t *testing.T) {
	p := newPty(t)
	s := blockedOnDialog(t, p, permissionDialog("Read file", "a.txt"))

	if pend := pendingOf(t, p, s); pend != nil {
		t.Fatalf("Pending = %+v; want nil for a dialog gx may not approve", pend)
	}
	err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "anything"})
	if !errors.Is(err, agentrunner.ErrRequestMismatch) {
		t.Fatalf("Allow = %v; want ErrRequestMismatch", err)
	}
	if st, _ := p.Status(s); st.State != agentrunner.StateBlocked {
		t.Fatalf("State = %v; want still blocked", st.State)
	}
}

func TestPty_AllowWithRequestIDSendsEnter(t *testing.T) {
	p := newPty(t)
	s := blockedOnDialog(t, p, permissionDialog("Bash command", "rg foo"))
	id := pendingOf(t, p, s).ID

	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: id}); err != nil {
		t.Fatal(err)
	}
	if st, err := p.Status(s); err != nil || st.State != agentrunner.StateWorking {
		t.Fatalf("Status = %+v, %v; want working after Enter", st, err)
	}
}

func TestPty_ChangedDialogRefusesBeforeAnyKey(t *testing.T) {
	p := newPty(t)
	s := blockedOnDialog(t, p, permissionDialog("Bash command", "rg foo"))
	seen := pendingOf(t, p, s).ID

	// The next dialog replaces the one the person saw.
	ptyControl(t, s, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: "permission prompt",
		Dialog: permissionDialog("Bash command", "rm -rf build")})
	deadline := time.Now().Add(5 * time.Second)
	for pend := pendingOf(t, p, s); pend == nil || pend.ID == seen; pend = pendingOf(t, p, s) {
		if time.Now().After(deadline) {
			t.Fatal("new dialog never drew")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, d := range []agentrunner.Decision{agentrunner.DecisionAllow, agentrunner.DecisionDeny} {
		err := p.Answer(s, agentrunner.Answer{Decision: d, RequestID: seen})
		if !errors.Is(err, agentrunner.ErrRequestMismatch) {
			t.Fatalf("%s = %v; want ErrRequestMismatch", d, err)
		}
	}
	pend := pendingOf(t, p, s)
	if pend == nil || pend.ID == seen {
		t.Fatalf("Pending = %+v; want the new dialog, still open", pend)
	}
}

func TestPty_DenyWithNoteReachesAgent(t *testing.T) {
	p := newPty(t)
	s := blockedOnDialog(t, p, permissionDialog("Edit file", "a.go"))
	id := pendingOf(t, p, s).ID

	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, RequestID: id, Text: "use rg"}); err != nil {
		t.Fatal(err)
	}
	if got := ptyLastPrompt(t, p, s); got != "use rg" {
		t.Fatalf("last prompt = %q, want the note", got)
	}
}
