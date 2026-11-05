package agentfake

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/testutil"
)

// Test helpers shared by this package's tests and the fake Codex's. They live
// outside _test.go files because a child package cannot read the parent's.

// capturingKind runs the fake harness and hands the test the holder's
// watcher.
const capturingKind agentrunner.Kind = "agentfake-capturing"

type capturingHarness struct {
	Harness
	watchers chan harness.Watcher
}

func (h capturingHarness) Watch(pid int, spawned time.Time, scr harness.Screen, stop <-chan struct{}) (harness.Watcher, error) {
	w, err := h.Harness.Watch(pid, spawned, scr, stop)
	h.watchers <- w
	return w, err
}

type HeldProgram struct {
	t       *testing.T
	Dir     string
	conv    string
	client  *holder.HolderClient
	Watcher harness.Watcher
	done    chan error
}

// HoldProgram runs cmd in a real holder. The holder starts its child from argv
// alone, so env reaches it through HolderOptions.Env rather than cmd.Env.
func HoldProgram(t *testing.T, cmd harness.Command, conv string, env ...string) *HeldProgram {
	t.Helper()
	progDir := NewProgramDir(t)
	env = append([]string{ProgramEnv + "=1", ProgramDirEnv + "=" + progDir}, env...)
	watchers := make(chan harness.Watcher, 1)
	// The holder's watch runs in this process, so it gets the dir directly.
	harness.Register(capturingKind, capturingHarness{Harness: Harness{Dir: progDir}, watchers: watchers})
	dir, err := os.MkdirTemp("", "gxh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	p := &HeldProgram{t: t, Dir: dir, conv: conv, done: make(chan error, 1)}
	go func() {
		p.done <- holder.RunHarnessHolder(dir, capturingKind, holder.HolderOptions{Env: env}, append([]string{cmd.Name}, cmd.Args...))
	}()
	select {
	case p.Watcher = <-watchers:
	case err := <-p.done:
		t.Fatalf("holder exited early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("holder never started a watch")
	}
	p.WaitFor("holder.sock answers", func() bool {
		p.client, err = holder.DialHolder(dir)
		return err == nil
	})
	t.Cleanup(func() {
		_ = p.client.Stop()
		<-p.done
	})
	return p
}

func (p *HeldProgram) WaitFor(what string, cond func() bool) {
	p.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			p.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func (p *HeldProgram) control(c Control) {
	p.t.Helper()
	if err := SendProgramControl(p.conv, c); err != nil {
		p.t.Fatal(err)
	}
}

// Control is control for tests outside this package.
func (p *HeldProgram) Control(c Control) {
	p.t.Helper()
	p.control(c)
}

// SendKeys types keys into the held program.
func (p *HeldProgram) SendKeys(keys string) {
	p.t.Helper()
	if err := p.client.SendKeys(keys); err != nil {
		p.t.Fatal(err)
	}
}

// WaitExit waits for the holder to exit. The cleanup waits on the same
// channel, so the result is put back.
func (p *HeldProgram) WaitExit() error {
	err := <-p.done
	p.done <- nil
	return err
}

func (p *HeldProgram) report(state, kind, msg string) {
	p.t.Helper()
	p.control(Control{Action: ActionReport, State: state, Kind: kind, Message: msg})
}

func (p *HeldProgram) WaitStatus(want harness.Status) {
	p.t.Helper()
	p.WaitFor(fmt.Sprintf("status %+v", want), func() bool { return p.Watcher.Status() == want })
}

func (p *HeldProgram) rows() []string {
	rows, _, err := p.client.Screen(0)
	if err != nil {
		p.t.Fatal(err)
	}
	return rows
}

func (p *HeldProgram) WaitScreen(text string) {
	p.t.Helper()
	p.WaitFor("the screen shows "+text, func() bool { return strings.Contains(strings.Join(p.rows(), "\n"), text) })
}

func NewConversationID(t *testing.T) string {
	return fmt.Sprintf("%s-%d", strings.ReplaceAll(t.Name(), "/", "-"), time.Now().UnixNano())
}

type SyncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *SyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *SyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// WorkerEnv is a repo to commit in and a gx stand-in that logs its arguments
// and stdin.
func WorkerEnv(t *testing.T) (cfg WorkerConfig, gxLog string) {
	t.Helper()
	repo := testutil.TempRepo(t)
	t.Chdir(repo)
	dir := t.TempDir()
	gxLog = filepath.Join(dir, "gx.log")
	gx := filepath.Join(dir, "gx")
	script := "#!/bin/sh\necho \"$@\" >> " + gxLog + "\ncat >> " + gxLog + "\n"
	if err := os.WriteFile(gx, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return WorkerConfig{Gx: gx}, gxLog
}

func CommitCount(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range strings.TrimSpace(string(out)) {
		n = n*10 + int(c-'0')
	}
	return n
}
