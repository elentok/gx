package codexdoctor

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil/agentfake"
)

// The test binary doubles as fakecodex, as agentfake's own tests do.
func TestMain(m *testing.M) {
	if os.Getenv(agentfake.CodexEnv) == "1" {
		agentfake.TUICodex()
	}
	os.Exit(m.Run())
}

// fakeEnv puts a `codex` script first on PATH. HOME and CODEX_HOME are temp
// dirs, so nothing reads the real ~/.codex.
func fakeEnv(t *testing.T, script string, extra ...string) []string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return append([]string{
		"PATH=" + bin + ":" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"CODEX_HOME=" + t.TempDir(),
	}, extra...)
}

const launchScript = `for a; do [ "$a" = debug ] && found=1; done
[ -n "$FAKE_REJECT" ] && { echo "error: unexpected argument '--approve-for-me'" >&2; exit 2; }
[ -n "$FAKE_OUT" ] && echo "$CODEX_HOME" > "$FAKE_OUT"
exit 0
`

func TestLaunchSession_AcceptedRunsDebugPromptInputInIsolatedHome(t *testing.T) {
	out := filepath.Join(t.TempDir(), "codex-home")
	env := fakeEnv(t, launchScript, "FAKE_OUT="+out)
	rec, err := NewDrivers(env).Session(LaunchSession)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Launch.Err != "" {
		t.Fatalf("Launch.Err = %q, want none", rec.Launch.Err)
	}
	if !slices.Contains(rec.Launch.Args, "--approve-for-me") {
		t.Errorf("Launch.Args = %v, want gx's launch args", rec.Launch.Args)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if home := strings.TrimSpace(string(got)); home == "" || home == env[2][len("CODEX_HOME="):] {
		t.Errorf("codex ran with CODEX_HOME %q, want an isolated one", home)
	}
	if detail, ok := checkLaunchFlags(Input{Recording: rec}); !ok {
		t.Errorf("checkLaunchFlags = %q, want pass", detail)
	}
}

func TestLaunchSession_RejectedNamesTheFlag(t *testing.T) {
	rec, err := NewDrivers(fakeEnv(t, launchScript, "FAKE_REJECT=1")).Session(LaunchSession)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Launch.Err == "" {
		t.Fatal("Launch.Err is empty, want the failed run")
	}
	detail, ok := checkLaunchFlags(Input{Recording: rec})
	if ok || !strings.Contains(detail, "--approve-for-me") {
		t.Errorf("checkLaunchFlags = %q, %v, want a fail naming --approve-for-me", detail, ok)
	}
}

func TestUnknownSessionIsSkipped(t *testing.T) {
	if _, err := NewDrivers(nil).Session("nope"); err != ErrNoSession {
		t.Errorf("err = %v, want ErrNoSession", err)
	}
}

// commitScript is a Codex that, once told a prompt, commits in its cwd the way
// the model's shell call would, then writes a finished rollout.
const commitScript = `case "$1" in --version) echo "codex-cli 0.166.0"; exit 0;; esac
printf '\033]7501;state=idle:app=codex\a'
read prompt
[ -z "$FAKE_NO_COMMIT" ] && git commit -q --allow-empty -m probe >/dev/null 2>&1
d="$CODEX_HOME/sessions/2026/01/01"
mkdir -p "$d"
printf '{"type":"event_msg","payload":{"type":"task_complete"}}\n' > "$d/rollout-x.jsonl"
exec sleep 30
`

func TestCommitSession_CommitsFromLinkedWorktreeAndCleansUp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	d := NewDrivers(fakeEnv(t, commitScript))
	d.Timeout = 20 * time.Second
	rec, err := d.Session(CommitSession)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Probe.Err != "" {
		t.Errorf("Probe.Err = %q, want none", rec.Probe.Err)
	}
	if len(rec.Frames) == 0 || len(rec.Rollouts) != 1 || rec.Rollouts[0].Name != "rollout-x.jsonl" {
		t.Errorf("frames %d, rollouts %v, want the pty output and one rollout", len(rec.Frames), rec.Rollouts)
	}
	left, _ := os.ReadDir(tmp)
	if len(left) != 0 {
		t.Errorf("temp dir keeps %v, want the repo and worktree removed", left)
	}
}

func TestCommitSession_FailsWhenNoCommitLands(t *testing.T) {
	d := NewDrivers(fakeEnv(t, commitScript, "FAKE_NO_COMMIT=1"))
	d.Timeout = 20 * time.Second
	rec, err := d.Session(CommitSession)
	if err != nil {
		t.Fatal(err)
	}
	if detail, ok := checkProbeCommit(Input{Recording: rec}); ok {
		t.Errorf("checkProbeCommit = %q, want a fail", detail)
	}
}

// fakecodexEnv makes `codex` the test binary acting as fakecodex. The returned
// func drives one turn the way a model would: it waits for the turn to start,
// then sends the given controls.
func fakecodexEnv(t *testing.T) (env []string, id, dir string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir = agentfake.NewProgramDir(t)
	id = "doctor-" + strings.ReplaceAll(t.Name(), "/", "-")
	env = fakeEnv(t, "exec '"+exe+"' \"$@\"\n",
		agentfake.CodexEnv+"=1", agentfake.CodexIDEnv+"="+id, agentfake.ProgramDirEnv+"="+dir)
	return env, id, dir
}

func driveTurn(t *testing.T, id string, controls ...agentfake.Control) func(codexHome string) {
	return func(codexHome string) {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			rollouts, _ := readRollouts(codexHome)
			if len(rollouts) == 1 && strings.Contains(string(rollouts[0].Data), "task_started") {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		for _, c := range controls {
			if err := agentfake.SendProgramControl(id, c); err != nil {
				t.Error(err)
				return
			}
		}
	}
}

func TestTurnSession_RecordsFramesHooksAndRollout(t *testing.T) {
	env, id, _ := fakecodexEnv(t)
	d := NewDrivers(env)
	d.Timeout = 30 * time.Second
	d.afterPrompt = driveTurn(t, id,
		agentfake.Control{Action: agentfake.ActionToolCall, Tool: "Bash", Command: "echo gx-doctor"},
		agentfake.Control{Action: agentfake.ActionFinish})
	rec, err := d.Session(TurnSession)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "PreToolUse"} {
		if !slices.Contains(rec.Hooks, event) {
			t.Errorf("Hooks = %v, want %s", rec.Hooks, event)
		}
	}
	if detail, ok := checkOSC7501Title(Input{Recording: rec}); !ok {
		t.Errorf("osc7501-title = %q, want pass", detail)
	}
	if detail, ok := checkHooksFire(Input{Recording: rec}); !ok {
		t.Errorf("hooks-fire = %q, want pass", detail)
	}
	// fakecodex's function_call lines lack the call_id real Codex writes, so
	// the reader check is not run on them.
	if len(rec.Rollouts) != 1 {
		t.Errorf("Rollouts = %d, want the turn's one rollout", len(rec.Rollouts))
	}
	live, err := d.Session(LiveRolloutsSession)
	if err != nil {
		t.Fatal(err)
	}
	if len(live.Rollouts) == 0 {
		t.Error("live-rollouts is empty, want the turn's rollout")
	}
}

func TestTurnSession_ErrorsWhenTheTurnNeverCompletes(t *testing.T) {
	env, _, _ := fakecodexEnv(t)
	d := NewDrivers(env)
	d.Timeout = 3 * time.Second
	if _, err := d.Session(TurnSession); err == nil || !strings.Contains(err.Error(), "task_complete") {
		t.Errorf("err = %v, want a missing task_complete", err)
	}
}

func TestDenySession_HonouredWhenHookDeniesAndNothingIsWritten(t *testing.T) {
	env, id, _ := fakecodexEnv(t)
	d := NewDrivers(env)
	d.Timeout = 30 * time.Second
	d.afterPrompt = driveTurn(t, id, agentfake.Control{Action: agentfake.ActionPermission, Command: "touch x"})
	rec, err := d.Session(DenySession)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(rec.Hooks, "PermissionRequest") || !rec.DenyHonoured {
		t.Errorf("Hooks = %v, DenyHonoured = %v, want a held deny", rec.Hooks, rec.DenyHonoured)
	}
}

func TestDenySession_NotHonouredWhenPermissionNeverAsked(t *testing.T) {
	env, id, _ := fakecodexEnv(t)
	d := NewDrivers(env)
	d.Timeout = 30 * time.Second
	d.afterPrompt = driveTurn(t, id, agentfake.Control{Action: agentfake.ActionFinish})
	rec, err := d.Session(DenySession)
	if err != nil {
		t.Fatal(err)
	}
	if rec.DenyHonoured {
		t.Error("DenyHonoured = true without a PermissionRequest")
	}
	if detail, ok := checkPermissionDeny(Input{Recording: rec}); ok {
		t.Errorf("permission-deny = %q, want a fail", detail)
	}
}
