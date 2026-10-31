package agentfake

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/codexdoctor"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

const codexFixtureRollout = "../../docs/specs/codex-support-fixtures/rollouts/plain-turn.jsonl"

// codexLaunchArgs is the shape the Codex harness launches with.
var codexLaunchArgs = []string{
	"-c", "check_for_update_on_startup=false",
	"--sandbox", "workspace-write",
	"--ask-for-approval", "on-request",
	"--approve-for-me",
	"-c", "sandbox_workspace_write.network_access=true",
	"--dangerously-bypass-hook-trust",
	"--model", "gpt-6",
}

type codexProc struct {
	t     *testing.T
	id    string
	home  string
	cwd   string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	out   *syncBuffer
	done  chan error
}

// startCodex runs the fake as a plain process, as the PTY runner would.
func startCodex(t *testing.T, home, id string, args ...string) *codexProc {
	t.Helper()
	return startCodexIn(t, t.TempDir(), home, id, args...)
}

// startCodexIn is startCodex in a given working directory.
func startCodexIn(t *testing.T, dir, home, id string, args ...string) *codexProc {
	t.Helper()
	cwd, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &codexProc{t: t, id: id, home: home, cwd: cwd, out: &syncBuffer{}, done: make(chan error, 1)}
	p.cmd = exec.Command(os.Args[0], args...)
	p.cmd.Dir = cwd
	p.cmd.Env = append(os.Environ(), CodexEnv+"=1", CodexIDEnv+"="+id, "CODEX_HOME="+home, ProgramDirEnv+"="+NewProgramDir(t))
	p.cmd.Stdout, p.cmd.Stderr = p.out, p.out
	if p.stdin, err = p.cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { p.done <- p.cmd.Wait() }()
	t.Cleanup(func() { _ = p.cmd.Process.Kill() })
	return p
}

func (p *codexProc) waitOut(text string) {
	p.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !strings.Contains(p.out.String(), text); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			p.t.Fatalf("timed out waiting for %q in %q", text, p.out.String())
		}
	}
}

// turnAndExit types prompt, ends the turn and exits the fake cleanly.
func (p *codexProc) turnAndExit(prompt string) {
	p.t.Helper()
	p.waitOut("state=idle")
	if _, err := io.WriteString(p.stdin, prompt+"\r"); err != nil {
		p.t.Fatal(err)
	}
	p.waitOut("state=working")
	if err := SendProgramControl(p.id, Control{Action: ActionFinish}); err != nil {
		p.t.Fatal(err)
	}
	p.waitOut("state=done")
	if err := SendProgramControl(p.id, Control{Action: ActionExit}); err != nil {
		p.t.Fatal(err)
	}
	if err := <-p.done; err != nil {
		p.t.Fatalf("fakecodex: %v\n%s", err, p.out.String())
	}
}

func rolloutFiles(t *testing.T, home string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func rolloutPairs(t *testing.T, path string) map[[2]string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pairs := map[[2]string]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var l struct {
			Type    string `json:"type"`
			Payload struct {
				Type string `json:"type"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &l); err != nil {
			t.Fatalf("bad rollout line %q: %v", line, err)
		}
		pairs[[2]string{l.Type, l.Payload.Type}] = true
	}
	return pairs
}

func TestCodex_WatcherFollowsATurnTitleAndExit(t *testing.T) {
	home := t.TempDir()
	conv := newConversationID(t)
	cmd := harness.Command{Name: os.Args[0], Args: codexLaunchArgs}
	p := holdProgram(t, cmd, conv, CodexEnv+"=1", CodexIDEnv+"="+conv, "CODEX_HOME="+home)

	p.waitStatus(harness.Status{State: harness.Idle})
	p.waitFor("the conversation id", func() bool { return p.watcher.ConversationID() == conv })
	if err := p.client.SendKeys("fix it\r"); err != nil {
		t.Fatal(err)
	}
	p.waitStatus(harness.Status{State: harness.Working})
	p.control(Control{Action: ActionFinish})
	p.waitStatus(harness.Status{State: harness.Idle})

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := "fix it | " + filepath.Base(cwd)
	p.waitFor("the display name", func() bool {
		raw, err := os.ReadFile(filepath.Join(p.dir, nativerunner.RawLogFile))
		name, ok := Harness{}.DisplayName(raw)
		return err == nil && ok && name == want
	})

	p.control(Control{Action: ActionExit, Code: 3})
	if err := <-p.done; err != nil {
		t.Fatal(err)
	}
	p.done <- nil
	code, err := os.ReadFile(filepath.Join(p.dir, nativerunner.ExitCodeFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(code)); got != "3" {
		t.Errorf("exit code = %q; want 3", got)
	}
}

func TestCodex_ReportsFollowTheRecordedStreamOrder(t *testing.T) {
	home := t.TempDir()
	p := startCodex(t, home, "order-1", codexLaunchArgs...)
	p.turnAndExit("Reply with pong")

	got := p.out.String()
	dir := filepath.Base(p.cwd)
	var last int
	for _, frame := range []string{
		"\x1b]0;" + dir + "\a",
		"state=idle:app=codex",
		"\x1b]0;" + codexSpinner + " " + dir + "\a",
		"state=working:app=codex",
		"\x1b]0;" + dir + "\a",
		"state=done:app=codex",
		"\x1b]0;Reply with pong | " + dir + "\a",
		"state=idle:app=codex",
		"\x1b]0;\a",
		"state=clear",
	} {
		i := strings.Index(got[last:], frame)
		if i < 0 {
			t.Fatalf("frame %q missing after offset %d in %q", frame, last, got)
		}
		last += i + len(frame)
	}
}

func TestCodex_InputRowIsCodexMarkerAndEmptyReadsAsEmpty(t *testing.T) {
	var buf bytes.Buffer
	box := newInputBox(&buf, "")
	box.marker = codexMarker
	box.draw()
	rows := strings.Split(strings.TrimPrefix(buf.String(), "\x1b[2J\x1b[H"), "\r\n")
	typing, err := harness.LookupTyping(harness.Codex)
	if err != nil {
		t.Fatal(err)
	}
	if got := typing.InputIsEmpty(rows); got != harness.InputEmpty {
		t.Errorf("empty input = %v; want InputEmpty", got)
	}
	for _, k := range []byte("hi") {
		buf.Reset()
		box.key(k)
	}
	rows = strings.Split(strings.TrimPrefix(buf.String(), "\x1b[2J\x1b[H"), "\r\n")
	if got := typing.InputIsEmpty(rows); got != harness.InputNotEmpty {
		t.Errorf("typed input = %v; want InputNotEmpty", got)
	}
}

func TestCodex_RolloutIsReadByProductionReaders(t *testing.T) {
	home := t.TempDir()
	p := startCodex(t, home, "readers-1", codexLaunchArgs...)
	p.turnAndExit("hello")

	stats, ok, err := codexsession.ReadStatsIn(home, p.cwd, p.id)
	if err != nil || !ok {
		t.Fatalf("ReadStatsIn = %+v, %v, %v", stats, ok, err)
	}
	if stats.TotalTokens != codexTurnTokens {
		t.Errorf("TotalTokens = %d; want %d", stats.TotalTokens, codexTurnTokens)
	}
	tokens, ok, err := codexsession.LastContextTokensIn(home, p.cwd, p.id)
	if err != nil || !ok || tokens != codexTurnTokens {
		t.Errorf("LastContextTokensIn = %d, %v, %v; want %d", tokens, ok, err, codexTurnTokens)
	}
	if ok, err := codexsession.VerifyIdentityIn(home, p.cwd, p.id); err != nil || !ok {
		t.Errorf("VerifyIdentityIn = %v, %v; want true", ok, err)
	}
	if ok, err := codexsession.VerifyIdentityIn(home, t.TempDir(), p.id); err != nil || ok {
		t.Errorf("VerifyIdentityIn for another cwd = %v, %v; want false", ok, err)
	}
}

func TestCodex_RolloutPassesTheDoctorRolloutCheck(t *testing.T) {
	home := t.TempDir()
	p := startCodex(t, home, "doctor-1", codexLaunchArgs...)
	p.turnAndExit("hello")

	files := rolloutFiles(t, home)
	if len(files) != 1 {
		t.Fatalf("rollouts = %v; want one", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if problem := codexdoctor.CheckRollout(data); problem != "" {
		t.Errorf("CheckRollout on the fake's rollout: %s", problem)
	}
}

func TestCodex_RolloutShapeMatchesTheRecordedFixture(t *testing.T) {
	home := t.TempDir()
	p := startCodex(t, home, "shape-1", codexLaunchArgs...)
	p.turnAndExit("hello")

	files := rolloutFiles(t, home)
	if len(files) != 1 {
		t.Fatalf("rollouts = %v; want one", files)
	}
	recorded := rolloutPairs(t, codexFixtureRollout)
	written := rolloutPairs(t, files[0])
	for _, want := range [][2]string{
		{"session_meta", ""}, {"event_msg", "task_started"}, {"response_item", "message"},
		{"event_msg", "token_count"}, {"event_msg", "task_complete"},
	} {
		if !written[want] {
			t.Errorf("rollout has no %v line", want)
		}
	}
	for pair := range written {
		if !recorded[pair] {
			t.Errorf("rollout writes %v, which the recorded fixture lacks", pair)
		}
	}
}

func TestCodex_VersionIsFixed(t *testing.T) {
	cmd := exec.Command(os.Args[0], "--version")
	cmd.Env = append(os.Environ(), CodexEnv+"=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "codex-cli 0.166.0" {
		t.Errorf("--version = %q; want codex-cli 0.166.0", got)
	}
}

func TestCodex_UnknownFlagIsRejected(t *testing.T) {
	cmd := exec.Command(os.Args[0], "--no-such-flag")
	cmd.Env = append(os.Environ(), CodexEnv+"=1", "CODEX_HOME="+t.TempDir())
	if err := cmd.Run(); err == nil {
		t.Error("an unknown flag exited 0")
	}
}

func TestCodex_ArgvIsRecorded(t *testing.T) {
	home := t.TempDir()
	p := startCodex(t, home, "argv-1", codexLaunchArgs...)
	p.waitOut("state=idle")
	got, err := ReadCodexArgs(home, p.id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != strings.Join(codexLaunchArgs, " ") {
		t.Errorf("argv = %q; want %q", got, codexLaunchArgs)
	}
}

func TestCodex_ResumeKeepsTheIDAndAppendsToTheRollout(t *testing.T) {
	home := t.TempDir()
	first := startCodex(t, home, "resume-1", codexLaunchArgs...)
	first.turnAndExit("one")
	// The ready file outlives its program, and a sender takes the first dir that has one.
	_ = os.Remove(programReadyPath(testDir("resume-1"), "resume-1"))

	second := startCodex(t, home, "ignored", append([]string{"resume", "resume-1"}, codexLaunchArgs...)...)
	second.id = "resume-1"
	second.turnAndExit("two")

	files := rolloutFiles(t, home)
	if len(files) != 1 {
		t.Fatalf("rollouts = %v; want one shared file", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), `"type":"session_meta"`); n != 1 {
		t.Errorf("session_meta lines = %d; want 1", n)
	}
	if n := strings.Count(string(data), `"type":"task_complete"`); n != 2 {
		t.Errorf("task_complete lines = %d; want 2", n)
	}
	if !strings.Contains(string(data), `"text":"one"`) || !strings.Contains(string(data), `"text":"two"`) {
		t.Errorf("rollout lacks a prompt: %s", data)
	}
	args, err := ReadCodexArgs(home, "resume-1")
	if err != nil || args[0] != "resume" {
		t.Errorf("recorded argv = %q, %v; want the resume launch", args, err)
	}
}
