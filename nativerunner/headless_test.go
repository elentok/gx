package nativerunner_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"golang.org/x/sys/unix"
)

// The test binary doubles as a fake claude: launched with fakeClaudeEnv set,
// it replays canned stream-json events instead of running tests.
const (
	fakeClaudeEnv    = "GX_FAKE_CLAUDE"
	fakeClaudeRecord = "GX_FAKE_CLAUDE_RECORD"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeClaudeEnv) == "1" {
		fakeClaude()
		return
	}
	os.Exit(m.Run())
}

type launchRecord struct {
	Args []string `json:"args"`
	Env  []string `json:"env"`
	Cwd  string   `json:"cwd"`
}

// fakeClaude answers each stdin message with one turn. "hang" starts a turn
// that never finishes, "block" waits on a permission prompt, "exit" quits.
func fakeClaude() {
	cwd, _ := os.Getwd()
	rec, _ := json.Marshal(launchRecord{Args: os.Args[1:], Env: os.Environ(), Cwd: cwd})
	_ = os.WriteFile(os.Getenv(fakeClaudeRecord), rec, 0o644)

	sessionID := os.Args[slices.Index(os.Args, "--session-id")+1]
	fmt.Println("claude: some stray log line")
	initSent := false
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var msg struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		_ = json.Unmarshal(scanner.Bytes(), &msg)
		if msg.Message.Content == "exit" {
			fmt.Println(`{"type":"result","subtype":"success"}`)
			os.Exit(0)
		}
		if !initSent {
			fmt.Printf(`{"type":"system","subtype":"init","session_id":%q}`+"\n", sessionID)
			initSent = true
		}
		if msg.Message.Content == "block" {
			fmt.Println(`{"type":"system","subtype":"session_state_changed","state":"running"}`)
			fmt.Println(`{"type":"system","subtype":"session_state_changed","state":"requires_action"}`)
			continue
		}
		fmt.Println(`{"type":"brand_new_event","x":1}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`)
		if msg.Message.Content != "hang" {
			fmt.Println(`{"type":"result","subtype":"success"}`)
		}
	}
}

type harness struct {
	runner *nativerunner.Headless
	root   string
	cwd    string
	record string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		root:   t.TempDir(),
		cwd:    t.TempDir(),
		record: filepath.Join(t.TempDir(), "launch.json"),
	}
	h.runner = &nativerunner.Headless{
		Root:         h.root,
		Claude:       exe,
		PollInterval: 5 * time.Millisecond,
		StopGrace:    2 * time.Second,
	}
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "cli")
	t.Setenv("CLAUDE_CONFIG_DIR", "/keep/me")
	return h
}

func (h *harness) start(t *testing.T, label string) agentrunner.Session {
	t.Helper()
	s, err := h.runner.Start(agentrunner.StartOptions{
		Label: label,
		Epic:  "epic",
		Cwd:   h.cwd,
		Kind:  "claude",
		Args:  []string{"--model", "opus"},
		Env:   []string{fakeClaudeEnv + "=1", fakeClaudeRecord + "=" + h.record},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = h.runner.Stop(s) })
	return s
}

func (h *harness) launch(t *testing.T) launchRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(h.record)
		if err == nil && len(data) > 0 {
			var rec launchRecord
			if err := json.Unmarshal(data, &rec); err != nil {
				t.Fatal(err)
			}
			return rec
		}
		if time.Now().After(deadline) {
			t.Fatal("fake claude never recorded its launch")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func envValue(env []string, name string) (string, bool) {
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == name {
			return v, true
		}
	}
	return "", false
}

func TestHeadless_StartLaunchesDetachedClaude(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	st, err := h.runner.Status(s)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != agentrunner.StateIdle || st.SessionID != s.SessionID || s.SessionID == "" {
		t.Fatalf("status after Start = %+v, session %+v", st, s)
	}

	rec := h.launch(t)
	wantArgs := []string{
		"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--session-id", s.SessionID, "--permission-mode", "auto", "--permission-prompt-tool", "stdio",
		"--model", "opus",
	}
	if !slices.Equal(rec.Args, wantArgs) {
		t.Errorf("args = %q\nwant %q", rec.Args, wantArgs)
	}
	if v, _ := envValue(rec.Env, "DISABLE_AUTOUPDATER"); v != "1" {
		t.Errorf("DISABLE_AUTOUPDATER = %q, want 1", v)
	}
	for _, gone := range []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT"} {
		if _, ok := envValue(rec.Env, gone); ok {
			t.Errorf("%s leaked into claude's env", gone)
		}
	}
	if v, _ := envValue(rec.Env, "CLAUDE_CONFIG_DIR"); v != "/keep/me" {
		t.Errorf("CLAUDE_CONFIG_DIR = %q, want it kept", v)
	}
	if got, _ := filepath.EvalSymlinks(rec.Cwd); got != mustEval(t, h.cwd) {
		t.Errorf("cwd = %q, want %q", rec.Cwd, h.cwd)
	}

	dir := filepath.Join(h.root, "epic-07")
	data, err := os.ReadFile(filepath.Join(dir, nativerunner.MetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta nativerunner.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Runner != "headless" || meta.SessionID != s.SessionID || meta.Label != "epic-07" ||
		meta.Epic != "epic" || meta.Cwd != h.cwd || meta.PID == 0 {
		t.Errorf("meta = %+v", meta)
	}
	// Setsid makes claude the leader of its own session, so a launchd stop of
	// the server's process group does not reach it.
	if sid, err := unix.Getsid(meta.PID); err != nil || sid != meta.PID {
		t.Errorf("getsid(%d) = %d, %v; want own session", meta.PID, sid, err)
	}
	if fi, err := os.Stat(filepath.Join(dir, nativerunner.StdinFile)); err != nil || fi.Mode()&os.ModeNamedPipe == 0 {
		t.Errorf("stdin is not a FIFO: %v %v", fi, err)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestHeadless_PromptRunsATurnAndSkipsUnknownLines(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	for turn := 1; turn <= 2; turn++ {
		if err := h.runner.Prompt(s, "hello"); err != nil {
			t.Fatal(err)
		}
		st := h.waitTurn(t, s, turn)
		if st.SessionID != s.SessionID {
			t.Fatalf("after prompt %d: status = %+v", turn, st)
		}
	}

	out, err := os.ReadFile(filepath.Join(h.root, "epic-07", nativerunner.OutFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "stray log line") || !strings.Contains(string(out), "brand_new_event") {
		t.Errorf("out.jsonl is missing claude's raw output:\n%s", out)
	}
}

func TestHeadless_WaitTimesOutWhileWorking(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if err := h.runner.Prompt(s, "hang"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Wait(s, []agentrunner.State{agentrunner.StateWorking}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	st, err := h.runner.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 50*time.Millisecond)
	if !errors.Is(err, agentrunner.ErrTimeout) {
		t.Fatalf("Wait err = %v, want ErrTimeout", err)
	}
	if st.State != agentrunner.StateWorking || st.Turn != 1 {
		t.Errorf("status = %+v, want working turn 1", st)
	}
}

func TestHeadless_RequiresActionIsBlocked(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if err := h.runner.Prompt(s, "block"); err != nil {
		t.Fatal(err)
	}
	st, err := h.runner.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if st.Turn != 1 {
		t.Errorf("Turn = %d, want 1", st.Turn)
	}
	if err := h.runner.Prompt(s, "hello"); !errors.Is(err, agentrunner.ErrNotReady) {
		t.Errorf("Prompt while blocked = %v, want ErrNotReady", err)
	}
}

func TestHeadless_ExitedClaudeIsDone(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if err := h.runner.Prompt(s, "exit"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Wait(s, []agentrunner.State{agentrunner.StateDone}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := h.runner.Prompt(s, "again"); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Errorf("Prompt after exit = %v, want ErrNotDelivered", err)
	}
}

func TestHeadless_LabelsFindListAndStop(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if _, err := h.runner.Start(agentrunner.StartOptions{Label: "epic-07", Cwd: h.cwd}); !errors.Is(err, agentrunner.ErrLabelTaken) {
		t.Fatalf("second Start = %v, want ErrLabelTaken", err)
	}
	if got, ok, err := h.runner.Find("epic-07"); err != nil || !ok || got != s {
		t.Errorf("Find = %+v %v %v", got, ok, err)
	}
	if got, err := h.runner.List("epic"); err != nil || !slices.Equal(got, []agentrunner.Session{s}) {
		t.Errorf("List(epic) = %+v %v", got, err)
	}
	if got, _ := h.runner.List("other"); len(got) != 0 {
		t.Errorf("List(other) = %+v", got)
	}

	pid := h.pid(t)
	if err := h.runner.Stop(s); err != nil {
		t.Fatal(err)
	}
	if err := h.runner.Stop(s); err != nil {
		t.Errorf("second Stop = %v, want nil", err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Errorf("claude pid %d still alive after Stop: %v", pid, err)
	}
	if _, err := h.runner.Status(s); !errors.Is(err, agentrunner.ErrNotFound) {
		t.Errorf("Status after Stop = %v, want ErrNotFound", err)
	}
	if _, ok, _ := h.runner.Find("epic-07"); ok {
		t.Error("Find still sees a stopped session")
	}
}

// waitTurn waits for turn to have finished. Waiting on idle alone isn't
// enough: the session is idle before the tailer sees the turn start.
func (h *harness) waitTurn(t *testing.T, s agentrunner.Session, turn int) agentrunner.Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, err := h.runner.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if st.Turn >= turn {
			if st.Turn != turn {
				t.Fatalf("Turn = %d, want %d", st.Turn, turn)
			}
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("turn %d never finished: %+v", turn, st)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *harness) pid(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.root, "epic-07", nativerunner.MetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta nativerunner.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	return meta.PID
}
