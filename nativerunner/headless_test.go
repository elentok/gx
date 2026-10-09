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
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
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
	if os.Getenv(agentfake.ClaudeEnv) == "1" {
		agentfake.Claude()
	}
	os.Exit(m.Run())
}

type launchRecord struct {
	Args []string `json:"args"`
	Env  []string `json:"env"`
	Cwd  string   `json:"cwd"`
}

// fakeRateLimitReset is the reset time the fake reports for "ratelimit".
const fakeRateLimitReset = 1791403200

// bgDoneFile, once created in the agent's cwd, ends the fake's background
// task.
const bgDoneFile = "bg-done"

// fakeClaude answers each stdin message with one turn. "hang" starts a turn
// that never finishes, "block" waits on a permission prompt, "stall" is
// never started, "background" leaves a background task running until
// bgDoneFile appears, "ratelimit"/"allowed" set the rate limit, "/compact"
// compacts without a command lifecycle, and "exit" quits.
func fakeClaude() {
	cwd, _ := os.Getwd()
	rec, _ := json.Marshal(launchRecord{Args: os.Args[1:], Env: os.Environ(), Cwd: cwd})
	_ = os.WriteFile(os.Getenv(fakeClaudeRecord), rec, 0o644)

	var mu sync.Mutex
	emit := func(lines ...string) {
		mu.Lock()
		defer mu.Unlock()
		for _, l := range lines {
			fmt.Println(l)
		}
	}

	sessionID := os.Args[slices.Index(os.Args, "--session-id")+1]
	emit("claude: some stray log line")
	initSent := false
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var msg struct {
			Type      string `json:"type"`
			UUID      string `json:"uuid"`
			RequestID string `json:"request_id"`
			Message   struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		_ = json.Unmarshal(scanner.Bytes(), &msg)
		if msg.Type == "control_request" {
			emit(fmt.Sprintf(`{"type":"control_response","response":{"subtype":"success","request_id":%q}}`, msg.RequestID),
				`{"type":"result","subtype":"error_during_execution"}`)
			continue
		}
		content := msg.Message.Content
		if content == "stall" {
			continue
		}
		lifecycle := func(state string) string {
			return fmt.Sprintf(`{"type":"command_lifecycle","command_uuid":%q,"state":%q}`, msg.UUID, state)
		}
		if content == "nocaps" {
			// A claude without msg_lifecycle_v1 sends no command lifecycle.
			emit(fmt.Sprintf(`{"type":"system","subtype":"init","session_id":%q,"capabilities":["interrupt_receipt_v1"]}`, sessionID),
				`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`,
				`{"type":"result","subtype":"success"}`)
			continue
		}
		if content != "/compact" {
			emit(lifecycle("queued"), lifecycle("started"))
		}
		if content == "exit" {
			emit(`{"type":"result","subtype":"success"}`)
			os.Exit(0)
		}
		if !initSent {
			emit(fmt.Sprintf(`{"type":"system","subtype":"init","session_id":%q,"capabilities":["msg_lifecycle_v1"]}`, sessionID))
			initSent = true
		}
		switch content {
		case "block":
			emit(`{"type":"system","subtype":"session_state_changed","state":"running"}`,
				`{"type":"system","subtype":"session_state_changed","state":"requires_action"}`)
			continue
		case "hang":
			emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`,
				`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"auto"}}`)
			continue
		case "/compact":
			emit(`{"type":"system","subtype":"status","status":"compacting"}`,
				`{"type":"system","subtype":"status","status":null,"compact_result":"success"}`,
				`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"manual"}}`,
				`{"type":"user","message":{"content":"summary"},"isCompactSummary":true}`,
				`{"type":"result","subtype":"success","num_turns":0}`)
			continue
		case "ratelimit", "allowed":
			status := map[string]string{"ratelimit": "rejected", "allowed": "allowed"}[content]
			emit(fmt.Sprintf(`{"type":"rate_limit_event","rate_limit_info":{"status":%q,"resetsAt":%d}}`, status, fakeRateLimitReset))
		case "background":
			emit(`{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"b1"}]}`)
			go func() {
				for {
					if _, err := os.Stat(bgDoneFile); err == nil {
						emit(`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`)
						return
					}
					time.Sleep(5 * time.Millisecond)
				}
			}()
		}
		emit(`{"type":"brand_new_event","x":1}`,
			`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`,
			`{"type":"result","subtype":"success"}`,
			lifecycle("completed"))
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
		Root:           h.root,
		Claude:         exe,
		PollInterval:   5 * time.Millisecond,
		StopGrace:      2 * time.Second,
		InterruptGrace: 2 * time.Second,
		PromptTimeout:  5 * time.Second,
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

func TestHeadless_PromptNeverStartedIsNotDelivered(t *testing.T) {
	h := newHarness(t)
	h.runner.PromptTimeout = 100 * time.Millisecond
	s := h.start(t, "epic-07")

	if err := h.runner.Prompt(s, "stall"); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("Prompt = %v, want ErrNotDelivered", err)
	}
}

func TestHeadless_InitWithoutRequiredCapabilityFailsPrompt(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	for range 2 {
		err := h.runner.Prompt(s, "nocaps")
		if !errors.Is(err, agentrunner.ErrMissingCapability) || !strings.Contains(err.Error(), "msg_lifecycle_v1") {
			t.Fatalf("Prompt = %v, want ErrMissingCapability naming msg_lifecycle_v1", err)
		}
	}
}

func TestHeadless_InterruptEndsTurnAndKeepsSession(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if err := h.runner.Prompt(s, "hang"); err != nil {
		t.Fatal(err)
	}
	// The hanging turn includes an auto compaction, which must not end it.
	if _, err := h.runner.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 100*time.Millisecond); !errors.Is(err, agentrunner.ErrTimeout) {
		t.Fatalf("Wait idle mid-turn = %v, want ErrTimeout", err)
	}
	if err := h.runner.Interrupt(s); err != nil {
		t.Fatal(err)
	}
	h.waitTurn(t, s, 1)
	if err := h.runner.Prompt(s, "hello"); err != nil {
		t.Fatalf("Prompt after Interrupt: %v", err)
	}
	h.waitTurn(t, s, 2)
	if !strings.Contains(h.out(t), `"control_response"`) {
		t.Error("claude never answered the interrupt control request")
	}
}

func TestHeadless_CompactRunsInSameSession(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if err := h.runner.Prompt(s, "hang"); err != nil {
		t.Fatal(err)
	}
	if err := h.runner.Interrupt(s); err != nil {
		t.Fatal(err)
	}
	h.waitTurn(t, s, 1)
	if err := h.runner.Prompt(s, "/compact"); err != nil {
		t.Fatalf("Prompt /compact: %v", err)
	}
	if st := h.waitTurn(t, s, 2); st.SessionID != s.SessionID {
		t.Errorf("session after /compact = %q, want %q", st.SessionID, s.SessionID)
	}
}

func TestHeadless_RateLimitFromEvent(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if _, limited, err := h.runner.RateLimit(s); err != nil || limited {
		t.Fatalf("RateLimit on fresh agent = %v, %v", limited, err)
	}
	if err := h.runner.Prompt(s, "ratelimit"); err != nil {
		t.Fatal(err)
	}
	h.waitTurn(t, s, 1)
	resetAt, limited, err := h.runner.RateLimit(s)
	if err != nil || !limited || !resetAt.Equal(time.Unix(fakeRateLimitReset, 0)) {
		t.Fatalf("RateLimit = %v, %v, %v; want limited until %v", resetAt, limited, err, time.Unix(fakeRateLimitReset, 0))
	}
	if err := h.runner.Prompt(s, "allowed"); err != nil {
		t.Fatal(err)
	}
	h.waitTurn(t, s, 2)
	if _, limited, _ := h.runner.RateLimit(s); limited {
		t.Error("still limited after an allowed rate_limit_event")
	}
}

func TestHeadless_FinishWaitsForBackgroundTasks(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if err := h.runner.Prompt(s, "background"); err != nil {
		t.Fatal(err)
	}
	st, err := h.runner.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 200*time.Millisecond)
	if !errors.Is(err, agentrunner.ErrTimeout) || st.State != agentrunner.StateWorking {
		t.Fatalf("Wait idle with a background task = %+v, %v; want working, ErrTimeout", st, err)
	}
	if err := os.WriteFile(filepath.Join(h.cwd, bgDoneFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.waitTurn(t, s, 1)
}

func TestHeadless_StopInterruptsAWorkingAgentFirst(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if err := h.runner.Prompt(s, "hang"); err != nil {
		t.Fatal(err)
	}
	if err := h.runner.Stop(s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.out(t), `"control_response"`) {
		t.Error("Stop sent SIGTERM without interrupting the turn first")
	}
}

func (h *harness) out(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.root, "epic-07", nativerunner.OutFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
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

// blockingProcs holds Start inside its post-launch process lookup.
type blockingProcs struct {
	entered chan struct{}
	release chan struct{}
}

func (b blockingProcs) Lookup(pid int) (nativerunner.Process, bool, error) {
	close(b.entered)
	<-b.release
	return nativerunner.Process{Start: "x"}, true, nil
}

func TestHeadless_StatusDoesNotBlockOnAStartInProgress(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-01")
	b := blockingProcs{entered: make(chan struct{}), release: make(chan struct{})}
	h.runner.Procs = b

	done := make(chan error, 1)
	go func() {
		_, err := h.runner.Start(agentrunner.StartOptions{
			Label: "epic-02", Epic: "epic", Cwd: h.cwd,
			Env: []string{fakeClaudeEnv + "=1", fakeClaudeRecord + "=" + h.record},
		})
		done <- err
	}()
	<-b.entered

	status := make(chan error, 1)
	go func() { _, err := h.runner.Status(s); status <- err }()
	select {
	case err := <-status:
		if err != nil {
			t.Errorf("Status = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Status blocked while Start was in progress")
	}
	if _, err := h.runner.Start(agentrunner.StartOptions{Label: "epic-02", Cwd: h.cwd}); !errors.Is(err, agentrunner.ErrLabelTaken) {
		t.Errorf("Start of a label mid-start = %v, want ErrLabelTaken", err)
	}
	close(b.release)
	if err := <-done; err != nil {
		t.Errorf("Start = %v", err)
	}
}

func TestHeadless_FailedLaunchReleasesItsLabel(t *testing.T) {
	h := newHarness(t)
	h.runner.Claude = filepath.Join(h.cwd, "no-such-claude")
	if _, err := h.runner.Start(agentrunner.StartOptions{Label: "epic-03", Cwd: h.cwd}); err == nil {
		t.Fatal("Start succeeded with a missing claude")
	}
	if _, ok, _ := h.runner.Find("epic-03"); ok {
		t.Error("failed Start left a session")
	}
	if _, err := h.runner.Start(agentrunner.StartOptions{Label: "epic-03", Cwd: h.cwd}); errors.Is(err, agentrunner.ErrLabelTaken) {
		t.Error("failed Start left its label reserved")
	}
}
