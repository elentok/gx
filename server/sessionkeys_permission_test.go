package server_test

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

const (
	enterLogged = `"<enter>"` + "\n"
	escLogged   = `"<esc>"` + "\n"
)

// permissionDialog is a Bash-like card with the given option rows.
func permissionDialog(header string, options ...string) []string {
	rows := []string{"────────────────────", header, "  ls", "Do you want to proceed?"}
	rows = append(rows, options...)
	return append(rows, "Esc to cancel")
}

func (f promptFixture) block(t *testing.T, c agentfake.Control) {
	t.Helper()
	if err := agentfake.SendProgramControl(f.conv, c); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionBlocked })
}

// blockOn opens a dialog that Enter or Esc closes.
func (f promptFixture) blockOn(t *testing.T, rows []string) {
	t.Helper()
	f.block(t, agentfake.Control{Action: agentfake.ActionDialogBlock, Dialog: rows, Message: "run ls?"})
}

func (f promptFixture) permission(t *testing.T, decision, requestID string) server.SessionResult {
	t.Helper()
	return f.keys(t, server.SessionKeysRequest{Permission: decision, RequestID: requestID})
}

// nothingSent closes the dialog from the fake's side and sends a Tab: any key
// the refused answer sent would be logged ahead of it.
func (f promptFixture) nothingSent(t *testing.T) {
	t.Helper()
	report(t, f.conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	if res := f.keys(t, server.SessionKeysRequest{Key: "Tab"}); res.Refused {
		t.Fatalf("tab = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("<tab>")+"\n")
}

func TestSessionKeys_PermissionAllowSendsEnterAndDenySendsEsc(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ decision, want string }{
		"allow": {"allow", enterLogged},
		"deny":  {"deny", escLogged},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startPromptSessionWithGrace(t, 10*time.Second)
			rows := agentfake.BashPermissionDialog("ls")
			f.blockOn(t, rows)
			if res := f.permission(t, tc.decision, claudeui.ReadDialog(rows).ID); res.Refused {
				t.Fatalf("%s = %+v", tc.decision, res)
			}
			waitForSubmitted(t, f.conv, tc.want)
			waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
		})
	}
}

func TestSessionKeys_PermissionAllowReadsAnyHeaderWithAPlainYes(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, permissionDialog("Read file", "❯ 1. Yes", "  2. No"))
	if res := f.permission(t, "allow", ""); res.Refused {
		t.Fatalf("allow = %+v", res)
	}
	waitForSubmitted(t, f.conv, enterLogged)
}

func TestSessionKeys_PermissionAllowNeverPicksAStandingOption(t *testing.T) {
	t.Parallel()
	for name, rows := range map[string][]string{
		"cursor off option 1": permissionDialog("Bash command", "  1. Yes", "❯ 2. No"),
		"option 1 not plain":  permissionDialog("Bash command", "❯ 1. Yes, and don't ask again for ls commands", "  2. No"),
		"plan approval": {
			"────────────────────", "Ready to code?", "  the plan",
			"Claude has written up a plan and is ready to execute. Would you like to proceed?",
			"❯ 1. Yes, and use auto mode", "  2. No, keep planning",
		},
		"unreadable": {"something no one can read"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startPromptSessionWithGrace(t, 10*time.Second)
			f.blockOn(t, rows)
			if res := f.permission(t, "allow", ""); !res.Refused || res.Reason != server.ReasonNotApprovable {
				t.Fatalf("allow = %+v, want %s", res, server.ReasonNotApprovable)
			}
			f.nothingSent(t)
		})
	}
}

func TestSessionKeys_PermissionDenyWorksOnAScreenGxCannotRead(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, []string{"something no one can read"})
	if res := f.permission(t, "deny", ""); res.Refused {
		t.Fatalf("deny = %+v", res)
	}
	waitForSubmitted(t, f.conv, escLogged)
}

func TestSessionKeys_PermissionRequestChangedSendsNothing(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	f.blockOn(t, agentfake.BashPermissionDialog("ls"))
	for _, decision := range []string{"allow", "deny"} {
		if res := f.permission(t, decision, "not-the-dialog"); !res.Refused || res.Reason != server.ReasonRequestChanged {
			t.Fatalf("%s = %+v, want %s", decision, res, server.ReasonRequestChanged)
		}
	}
	f.nothingSent(t)
}

func TestSessionKeys_PermissionWhoseDialogStaysOpenIsNotDelivered(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	// Written by ActionWrite, so no key closes it.
	rows := agentfake.BashPermissionDialog("ls")
	if err := agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionWrite, Output: "\x1b[2J\x1b[H" + joinRows(rows)}); err != nil {
		t.Fatal(err)
	}
	f.block(t, agentfake.Control{Action: agentfake.ActionReport, State: "blocked", Kind: "permission"})
	waitForScreen(t, waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id)), "Do you want to proceed?")
	if res := f.permission(t, "allow", ""); !res.Refused || res.Reason != server.ReasonKeysNotDelivered {
		t.Fatalf("allow = %+v, want %s", res, server.ReasonKeysNotDelivered)
	}
}

func joinRows(rows []string) string {
	out := ""
	for i, r := range rows {
		if i > 0 {
			out += "\r\n"
		}
		out += r
	}
	return out
}

func TestSessionKeys_PermissionRefusals(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	for name, tc := range map[string]struct {
		req  server.SessionKeysRequest
		want string
	}{
		"not blocked":     {server.SessionKeysRequest{Permission: "allow"}, server.ReasonNotBlocked},
		"bad value":       {server.SessionKeysRequest{Permission: "maybe"}, server.ReasonSessionBadRequest},
		"with a key":      {server.SessionKeysRequest{Permission: "allow", Key: "Tab"}, server.ReasonSessionBadRequest},
		"unknown session": {server.SessionKeysRequest{ID: "nope", Permission: "allow"}, server.ReasonSessionUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			if got := f.keys(t, tc.req); !got.Refused || got.Reason != tc.want {
				t.Errorf("keys = %+v, want %s", got, tc.want)
			}
		})
	}
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("submitted = %q, want nothing sent", got)
	}
}

func TestSessionKeys_PermissionKeepsADesignCountdown(t *testing.T) {
	t.Parallel()
	for _, decision := range []string{"allow", "deny"} {
		t.Run(decision, func(t *testing.T) {
			t.Parallel()
			f := startKeysAdvancing(t)
			rows := agentfake.BashPermissionDialog("ls")
			if err := agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionDialogBlock, Dialog: rows, Message: "run ls?"}); err != nil {
				t.Fatal(err)
			}
			waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionBlocked })
			res, err := f.h.Client.KeysSession(f.ctx, server.SessionKeysRequest{ID: f.id, Permission: decision})
			if err != nil || res.Refused {
				t.Fatalf("%s = %+v, %v", decision, res, err)
			}
			if got := sessionSnapshot(t, f); got.AutoAdvanceAt == nil {
				t.Errorf("session = %+v, want the countdown running after %s", got, decision)
			}
		})
	}
}
