package servertest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// FirstPromptGrace is the short wait for a typed prompt's turn to start that
// the prompt tests give the server.
const FirstPromptGrace = 300 * time.Millisecond

// StartCodexPromptSession runs a session on the fake Codex, Idle, with
// extraEnv added to the env it runs in. The conversation id is per test: the
// fake's control FIFO is keyed by it, so parallel tests sharing one id would
// steal each other's controls.
func StartCodexPromptSession(t *testing.T, extraEnv ...string) PromptFixture {
	t.Helper()
	return StartCodexPromptSessionWithGrace(t, FirstPromptGrace, extraEnv...)
}

// StartCodexPromptSessionWithGrace is StartCodexPromptSession with a longer
// wait for an answer to take effect, for a test whose fake does real work first.
func StartCodexPromptSessionWithGrace(t *testing.T, grace time.Duration, extraEnv ...string) PromptFixture {
	t.Helper()
	conv := "thread-server-prompt-" + strings.ReplaceAll(t.Name(), "/", "-")
	bin := FakeCodexDir(t)
	home := t.TempDir()
	f := StartDesigns(t, func(c *server.Config) { c.PromptGrace = grace },
		WithChatHarness("codex"),
		WithEnv(append([]string{
			"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + home,
			"CODEX_HOME=" + filepath.Join(home, ".codex"),
			agentfake.CodexIDEnv + "=" + conv,
			agentfake.ProgramDirEnv + "=" + agentfake.NewProgramDir(t),
		}, extraEnv...)...))
	res, err := f.H.Client.CreateSession(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: "codex"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	WaitForHolder(t, filepath.Join(f.H.SessionsDir, id))
	WaitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	return PromptFixture{DesignsFixture: f, ID: id, Conv: conv}
}

// StartCodexPermissionSession is a Codex session with its own conversation id,
// because the fake's control channel is named by it and these tests run in
// parallel. withDialog makes the fake draw its own approval dialog.
func StartCodexPermissionSession(t *testing.T, withDialog bool) PromptFixture {
	t.Helper()
	conv := "thread-perm-" + strings.ReplaceAll(t.Name(), "/", "-")
	env := []string{agentfake.CodexIDEnv + "=" + conv}
	grace := FirstPromptGrace
	if withDialog {
		// An answer ends the fake's turn, which runs its hooks first.
		env = append(env, agentfake.CodexNoPermissionHookEnv+"=1")
		grace = 10 * time.Second
	}
	f := StartCodexPromptSessionWithGrace(t, grace, env...)
	f.Conv = conv
	if withDialog {
		// A deny ends a turn, so there must be one.
		if res := f.Prompt(t, "run it"); res.Refused {
			t.Fatalf("prompt = %+v", res)
		}
		WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	}
	return f
}

// SeedSession writes a session record with no holder behind it.
func SeedSession(t *testing.T, f PromptFixture, id, state string) {
	t.Helper()
	dir := filepath.Join(f.H.SessionsDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{"id": id, "harness": "claude", "project": "proj", "dir": t.TempDir(), "status": map[string]any{"state": state}})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Keys sends req to the session; an empty req.ID means the fixture's session.
func (f PromptFixture) Keys(t *testing.T, req wire.SessionKeysRequest) wire.SessionResult {
	t.Helper()
	if req.ID == "" {
		req.ID = f.ID
	}
	res, err := f.H.Client.KeysSession(f.Ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// KeysWeb posts the request to the web mux and returns the refusal reason.
func (f PromptFixture) KeysWeb(t *testing.T, req wire.SessionKeysRequest) string {
	t.Helper()
	if req.ID == "" {
		req.ID = f.ID
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	code, out := WebPostJSON(t, f.Base+"/v1/sessions/keys", string(body), f.Ck, "", JSONPost(f.Base))
	if code != 200 {
		t.Fatalf("web keys = %d %s", code, out)
	}
	return ReasonOf(t, out)
}

// Block sends c to the fake and waits for the session to report Blocked.
func (f PromptFixture) Block(t *testing.T, c agentfake.Control) {
	t.Helper()
	if err := agentfake.SendProgramControl(f.Conv, c); err != nil {
		t.Fatal(err)
	}
	WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionBlocked })
}

// BlockOn opens a dialog that Enter or Esc closes.
func (f PromptFixture) BlockOn(t *testing.T, rows []string) {
	t.Helper()
	f.Block(t, agentfake.Control{Action: agentfake.ActionDialogBlock, Dialog: rows, Message: "run ls?"})
}

// BlockCodex opens Codex's approval dialog for command, which nothing but a
// key answers. An empty command leaves a dialog gx cannot read.
func (f PromptFixture) BlockCodex(t *testing.T, command string) {
	t.Helper()
	f.Block(t, agentfake.Control{Action: agentfake.ActionPermission, Command: command})
	// The fake reports Blocked before it draws the dialog.
	WaitForScreen(t, WaitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID)), "Blocked by hook")
}

// Permission answers the pending permission request with decision.
func (f PromptFixture) Permission(t *testing.T, decision, requestID string) wire.SessionResult {
	t.Helper()
	return f.Keys(t, wire.SessionKeysRequest{Permission: decision, RequestID: requestID})
}

// NothingSent closes the dialog from the fake's side and sends a Tab: any key
// the refused answer sent would be logged ahead of it.
func (f PromptFixture) NothingSent(t *testing.T) {
	t.Helper()
	Report(t, f.Conv, agentfake.Control{State: "idle"})
	WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if res := f.Keys(t, wire.SessionKeysRequest{Key: "Tab"}); res.Refused {
		t.Fatalf("tab = %+v", res)
	}
	WaitForSubmitted(t, f.Conv, strconv.Quote("<tab>")+"\n")
}
