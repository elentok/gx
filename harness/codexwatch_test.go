package harness_test

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/harness"
)

// watchCodex builds a watcher whose dialog check reads rows with the Codex
// harness's own classifier.
func watchCodex(t *testing.T, sessionFile string, rows []string) harness.TitleWatcher {
	t.Helper()
	h, err := harness.Lookup(harness.Codex)
	if err != nil {
		t.Fatal(err)
	}
	return harness.NewCodexWatcher(sessionFile, func() bool {
		return h.StartupScreen(nil, rows).Dialog != harness.NoDialog
	})
}

func osc(state, kind, msg string) harness.ProgramReport {
	return harness.ProgramReport{State: state, App: "codex", Kind: kind, Message: msg, Progress: -1}
}

func TestCodexWatcher_ReportsMapToStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		title  string
		report harness.ProgramReport
		want   harness.Status
	}{
		{"idle", "dir", osc("idle", "", ""), harness.Status{State: harness.Idle}},
		{"done", "dir", osc("done", "", ""), harness.Status{State: harness.Idle}},
		{"working", "⠋ dir", osc("working", "", ""), harness.Status{State: harness.Working}},
		{"working with title Ready is background", "Ready", osc("working", "", ""), harness.Status{State: harness.Working, Background: true}},
		{"permission", "dir", osc("blocked", "permission", "run ls"), harness.Status{State: harness.Blocked, Blocked: harness.BlockedPermission, Message: "run ls"}},
		{"question", "dir", osc("blocked", "question", "which?"), harness.Status{State: harness.Blocked, Blocked: harness.BlockedQuestion, Message: "which?"}},
		{"error", "dir", osc("error", "", "bad model"), harness.Status{State: harness.Blocked, Blocked: harness.BlockedError, Message: "bad model"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := watchCodex(t, "", nil)
			w.Title(tc.title)
			w.Program(tc.report)
			if got := w.Status(); got != tc.want {
				t.Fatalf("Status = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCodexWatcher_BackgroundIgnoresArrivalOrder(t *testing.T) {
	t.Parallel()
	w := watchCodex(t, "", nil)
	w.Program(osc("working", "", ""))
	w.Title("Ready")
	if got := w.Status(); !got.Background || got.State != harness.Working {
		t.Fatalf("Status = %+v, want Working with Background", got)
	}
	w.Title("⠋ gx-codex-rec")
	if got := w.Status(); got.Background {
		t.Fatalf("Status = %+v, want no Background once the title changes", got)
	}
}

func TestCodexWatcher_ClearKeepsStatus(t *testing.T) {
	t.Parallel()
	w := watchCodex(t, "", nil)
	w.Program(osc("working", "", ""))
	w.Program(osc("clear", "", ""))
	if got := w.Status().State; got != harness.Working {
		t.Fatalf("State after clear = %v, want Working", got)
	}
}

func TestCodexWatcher_TitleFallback(t *testing.T) {
	t.Parallel()
	w := watchCodex(t, "", nil)
	if got := w.Status().State; got != harness.Starting {
		t.Fatalf("State before any title = %v, want Starting", got)
	}
	w.Title("⠙ gx-codex-rec")
	if got := w.Status().State; got != harness.Working {
		t.Fatalf("State with spinner title = %v, want Working", got)
	}
	w.Title("gx-codex-rec")
	if got := w.Status().State; got != harness.Idle {
		t.Fatalf("State with plain title = %v, want Idle", got)
	}
	w.Program(osc("working", "", ""))
	w.Title("gx-codex-rec")
	if got := w.Status().State; got != harness.Working {
		t.Fatalf("a report must beat the title: State = %v, want Working", got)
	}
}

func TestCodexWatcher_ReadyAfterFirstIdleOnly(t *testing.T) {
	t.Parallel()
	w := watchCodex(t, "", nil)
	w.Title("dir")
	if got := w.Ready(); got != "" {
		t.Fatalf("Ready on the title alone = %q, want none", got)
	}
	w.Program(osc("working", "", ""))
	if got := w.Ready(); got != "" {
		t.Fatalf("Ready while working = %q, want none", got)
	}
	w.Program(osc("idle", "", ""))
	if got := w.Ready(); got != harness.ReadyOSC7501 {
		t.Fatalf("Ready after idle = %q, want %q", got, harness.ReadyOSC7501)
	}
	w.Program(osc("blocked", "permission", ""))
	if got := w.Ready(); got != "" {
		t.Fatalf("Ready while blocked = %q, want none", got)
	}
}

// errorRollout writes a session file and a rollout whose turn ended in an
// error, with an optional 100% sample resetting at resetsAt first.
func errorRollout(t *testing.T, kind string, resetsAt int64) (sessionFile, codexHome string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessionFile = filepath.Join(dir, "codex-session.json")
	if err := os.WriteFile(sessionFile, []byte(`{"session_id":"s1","cwd":"/repo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rollout := `{"type":"session_meta","payload":{"id":"s1","cwd":"/repo"}}` + "\n"
	if resetsAt != 0 {
		rollout += fmt.Sprintf(`{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":100,"resets_at":%d}}}}`+"\n", resetsAt)
	}
	rollout += fmt.Sprintf(`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"codex says no","codex_error_info":%q}}}`+"\n", kind)
	codexHome = filepath.Join(dir, "codex")
	path := filepath.Join(codexHome, "sessions", "2026", "01", "01", "rollout-s1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(rollout), 0o644); err != nil {
		t.Fatal(err)
	}
	return sessionFile, codexHome
}

func TestCodexWatcher_ErrorShowsTheRolloutsText(t *testing.T) {
	t.Parallel()
	sessionFile, home := errorRollout(t, "model_not_found", 0)
	w := harness.NewCodexWatcherIn(sessionFile, home, func() bool { return false })
	w.Program(osc("error", "", "osc text"))
	want := harness.Status{State: harness.Blocked, Blocked: harness.BlockedError, Message: "codex says no"}
	if got := w.Status(); got != want {
		t.Fatalf("Status = %+v, want %+v", got, want)
	}
}

func TestCodexWatcher_UsageLimitErrorIsRateLimitedWithItsReset(t *testing.T) {
	t.Parallel()
	resetsAt := time.Now().Add(time.Hour).Unix()
	sessionFile, home := errorRollout(t, "other", resetsAt)
	w := harness.NewCodexWatcherIn(sessionFile, home, func() bool { return false })
	w.Program(osc("error", "", ""))
	got := w.Status()
	if got.State != harness.RateLimited || got.ResetAt.Unix() != resetsAt || !strings.HasPrefix(got.Message, "resets ") {
		t.Fatalf("Status = %+v, want RateLimited resetting at %d", got, resetsAt)
	}
}

func TestCodexWatcher_DialogOnScreenBlocksUntilFirstReport(t *testing.T) {
	t.Parallel()
	w := watchCodex(t, "", []string{"Update available: Press Enter to continue"})
	w.Title("dir")
	want := harness.Status{State: harness.Blocked, Blocked: harness.BlockedDialog}
	if got := w.Status(); got != want {
		t.Fatalf("Status with a dialog up = %+v, want %+v", got, want)
	}
	w.Program(osc("idle", "", ""))
	if got := w.Status().State; got != harness.Idle {
		t.Fatalf("State after the idle report = %v, want Idle", got)
	}
}

func TestCodexWatcher_ConversationIDFromSessionFile(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), harness.CodexSessionFile)
	w := watchCodex(t, file, nil)
	if got := w.ConversationID(); got != "" {
		t.Fatalf("ConversationID before the hook ran = %q, want empty", got)
	}
	if err := os.WriteFile(file, []byte(`{"session_id":"thread-1","hook_event_name":"SessionStart"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := w.ConversationID(); got != "thread-1" {
		t.Fatalf("ConversationID = %q, want thread-1", got)
	}
}

func TestCodexStartupScreen_AnswersEveryDialog(t *testing.T) {
	t.Parallel()
	h, err := harness.Lookup(harness.Codex)
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]harness.Dialog{
		"Trust this directory? Press Enter to continue": harness.TrustDialog,
		"Hooks review: Press Enter to continue":         harness.HooksReviewDialog,
		"Update available: Press Enter to continue":     harness.UpdateDialog,
		"Model nudge: Press Enter to continue":          harness.ModelNudgeDialog,
		"› ask codex":                                   harness.NoDialog,
	} {
		got := h.StartupScreen(nil, []string{text})
		if got.Dialog != want || want != harness.NoDialog && got.Key == "" {
			t.Errorf("StartupScreen(%q) = %+v, want dialog %v with a key", text, got, want)
		}
	}
}

func TestCodexSessionStartHook_SavesPayloadForTheWatcher(t *testing.T) {
	t.Parallel()
	args := harness.CodexArgs(harness.CodexArgsInput{})
	if !slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "hooks.SessionStart=") }) {
		t.Fatalf("CodexArgs has no SessionStart hook: %v", args)
	}
	file := filepath.Join(t.TempDir(), harness.CodexSessionFile)
	cmd := exec.Command("sh", "-c", harness.CodexSessionStartCommand)
	cmd.Env = append(os.Environ(), harness.CodexSessionFileEnv+"="+file)
	cmd.Stdin = strings.NewReader(`{"session_id":"thread-7","hook_event_name":"SessionStart"}`)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook failed: %v\n%s", err, out)
	}
	if got := watchCodex(t, file, nil).ConversationID(); got != "thread-7" {
		t.Fatalf("ConversationID = %q, want thread-7", got)
	}
}

// codexStreamStates replays a recorded stream and returns the states the
// watcher showed after each frame, repeats dropped.
func codexStreamStates(t *testing.T, path string) []harness.State {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := watchCodex(t, "", nil)
	var parser harness.ProgramStatusParser
	var states []harness.State
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var frame struct {
			B64 string `json:"b64"`
		}
		if err := json.Unmarshal(sc.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		raw, err := base64.StdEncoding.DecodeString(frame.B64)
		if err != nil {
			t.Fatal(err)
		}
		if title, ok := screen.Title(raw); ok {
			w.Title(title)
		}
		_, reports := parser.Feed(raw)
		for _, r := range reports {
			w.Program(r)
		}
		if s := w.Status().State; len(states) == 0 || states[len(states)-1] != s {
			states = append(states, s)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return states
}

func TestCodexWatcher_PlainTurnStream(t *testing.T) {
	t.Parallel()
	got := codexStreamStates(t, "../docs/specs/codex-support-fixtures/streams/plain-turn.jsonl")
	// The recording has two turns. The first frames precede the first title.
	want := []harness.State{harness.Starting, harness.Idle, harness.Working, harness.Idle, harness.Working, harness.Idle}
	if !slices.Equal(got, want) {
		t.Fatalf("states = %v, want %v", got, want)
	}
}

func TestCodexWatcher_CountsEveryWorkingReportAsATurn(t *testing.T) {
	t.Parallel()
	w := watchCodex(t, "", nil)
	tw, ok := w.(harness.TurnWatcher)
	if !ok {
		t.Fatal("codex watcher is no TurnWatcher")
	}
	// A second working report with no change of state is a prompt sent to a
	// Background agent.
	for _, state := range []string{"idle", "working", "working", "idle", "working"} {
		w.Program(osc(state, "", ""))
	}
	if got := tw.Turns(); got != 3 {
		t.Errorf("Turns = %d, want 3", got)
	}
}
