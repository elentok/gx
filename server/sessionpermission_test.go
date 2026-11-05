package server_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

// permissionRead GETs the permission route on the web mux.
func permissionRead(t *testing.T, f servertest.PromptFixture, id string) (int, string) {
	t.Helper()
	return servertest.WebDoCookie(t, http.MethodGet, f.Base+"/v1/sessions/permission?session="+id, f.Ck)
}

func wantPermission(t *testing.T, f servertest.PromptFixture, want wire.SessionPermissionResponse) {
	t.Helper()
	code, body := permissionRead(t, f, f.ID)
	var got wire.SessionPermissionResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil || code != http.StatusOK {
		t.Fatalf("permission = %d %q (%v)", code, body, err)
	}
	if got != want {
		t.Errorf("permission = %+v, want %+v", got, want)
	}
}

func TestSessionPermission_ClaudeDialogs(t *testing.T) {
	t.Parallel()
	plan := []string{
		"────────────────────", "Ready to code?", "  the plan",
		"Claude has written up a plan and is ready to execute. Would you like to proceed?",
		"❯ 1. Yes, and use auto mode", "  2. No, keep planning",
	}
	bash := agentfake.BashPermissionDialog("ls")
	for name, tc := range map[string]struct {
		rows []string
		want wire.SessionPermissionResponse
	}{
		"bash": {bash, wire.SessionPermissionResponse{Kind: "prompt", Text: "Bash command\nls", RequestID: claudeui.ReadDialog(bash).ID}},
		"plan": {plan, wire.SessionPermissionResponse{Kind: "plan", Text: "Ready to code?\nthe plan", RequestID: claudeui.ReadDialog(plan).ID}},
		"unreadable": {
			[]string{"something no one can read"},
			wire.SessionPermissionResponse{Kind: "unreadable"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := servertest.StartPromptSession(t, servertest.FirstPromptGrace)
			f.BlockOn(t, tc.rows)
			wantPermission(t, f, tc.want)
		})
	}
}

func TestSessionPermission_CodexHookRequestGivesItsText(t *testing.T) {
	t.Parallel()
	f := servertest.StartCodexPermissionSession(t, false)
	dir := filepath.Join(f.H.SessionsDir, f.ID)
	if err := nativerunner.WritePermissionRequest(dir, nativerunner.PermissionRequest{ID: "req-1", Tool: "Bash", Description: "rg foo"}); err != nil {
		t.Fatal(err)
	}
	f.Block(t, agentfake.Control{Action: agentfake.ActionReport, State: "blocked", Kind: "permission"})
	wantPermission(t, f, wire.SessionPermissionResponse{Kind: "prompt", Text: "rg foo", RequestID: "req-1"})
}

func TestSessionPermission_CodexScreenOnly(t *testing.T) {
	t.Parallel()
	f := servertest.StartCodexPermissionSession(t, true)
	f.BlockCodex(t, "rg foo")
	code, body := permissionRead(t, f, f.ID)
	var got wire.SessionPermissionResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil || code != http.StatusOK {
		t.Fatalf("permission = %d %q (%v)", code, body, err)
	}
	if got.Kind != "prompt" || !strings.Contains(got.Text, "rg foo") || got.RequestID == "" {
		t.Errorf("permission = %+v, want a prompt with the command and an id", got)
	}
}

func TestSessionPermission_Refusals(t *testing.T) {
	t.Parallel()
	f := servertest.StartPromptSession(t, servertest.FirstPromptGrace)
	servertest.SeedSession(t, f, "seeded-exited", wire.SessionExited)
	for name, tc := range map[string]struct{ id, want string }{
		"not blocked": {f.ID, wire.ReasonNotBlocked},
		"exited":      {"seeded-exited", wire.ReasonSessionNotLive},
		"unknown":     {"nope", wire.ReasonSessionUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			code, body := permissionRead(t, f, tc.id)
			if code == http.StatusOK || servertest.ReasonOf(t, body) != tc.want {
				t.Errorf("permission = %d %s, want refusal %s", code, body, tc.want)
			}
		})
	}
}

func TestSessionPermission_QueueAgentIsReadOnly(t *testing.T) {
	t.Parallel()
	f := servertest.StartPromptSession(t, servertest.FirstPromptGrace)
	code, body := permissionRead(t, f, "queue-agent-x")
	if code == http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonSessionReadOnly {
		t.Errorf("permission = %d %s, want refusal %s", code, body, wire.ReasonSessionReadOnly)
	}
}
