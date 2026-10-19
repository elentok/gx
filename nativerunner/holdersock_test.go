package nativerunner_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
)

// echoChild prints a ready line, then echoes every line it reads.
const echoChild = `printf 'ready\n'; while read -r l; do printf 'got:%s\n' "$l"; done`

// startHolder runs a holder over child in-process and dials it.
func startHolder(t *testing.T, child string) (dir string, c *nativerunner.HolderClient, done <-chan error) {
	t.Helper()
	dir = shortTempDir(t)
	errc := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		errc <- nativerunner.RunHolder(dir, []string{"sh", "-c", child})
	}()
	waitFor(t, "holder.sock answers", func() bool {
		var err error
		c, err = nativerunner.DialHolder(dir)
		return err == nil
	})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	return dir, c, errc
}

func TestHolderSocket_CoreOps(t *testing.T) {
	dir, c, done := startHolder(t, echoChild)

	if c.Version() != nativerunner.HolderProtocolVersion {
		t.Errorf("version = %d; want %d", c.Version(), nativerunner.HolderProtocolVersion)
	}
	if !c.Supports(nativerunner.OpDenyReason) {
		t.Error("ping does not list deny-reason")
	}
	waitForScreen(t, c, "ready")
	if err := c.SendKeys("hi\r"); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, c, "got:hi")

	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("holder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, nativerunner.ExitCodeFile)); err != nil {
		t.Errorf("no exit code after stop: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, nativerunner.HolderSocketFile)); !os.IsNotExist(err) {
		t.Errorf("holder.sock left behind: %v", err)
	}
}

func TestHolderSocket_NoOptionalOps(t *testing.T) {
	nativerunner.SetHolderOptionalOps(t, nil)
	_, c, _ := startHolder(t, echoChild)

	if c.Supports(nativerunner.OpDenyReason) {
		t.Error("holder with no optional ops reports one supported")
	}
	if err := c.DenyWithReason("no"); err == nil {
		t.Error("holder without deny-reason served it")
	}
	waitForScreen(t, c, "ready")
}

// denyChild sits at a permission prompt; on the first key typed it runs tail.
const denyChild = `d="$HOME/.claude/sessions"; mkdir -p "$d"; f="$d/$$.json"; t=$(date +%s)000
st() { printf '{"pid":%s,"startedAt":%s,"status":"%s"%s}' $$ "$t" "$1" "$2" > "$f.tmp" && mv "$f.tmp" "$f"; }
st waiting ',"waitingFor":"permission prompt"'
stty -icanon min 1; printf 'ready\n'; dd bs=1 count=1 2>/dev/null >/dev/null; stty icanon
`

// Esc idles the prompt; the next line is the reason claude starts on.
const denyGoesIdle = `st idle; read -r l; st busy; printf 'got:%s\n' "$l"; cat >/dev/null`

const denyStaysWaiting = `cat >/dev/null`

func startDenyHolder(t *testing.T, tail string) *nativerunner.HolderClient {
	t.Helper()
	t.Setenv("HOME", shortTempDir(t))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	_, c, _ := startHolder(t, denyChild+tail)
	waitForScreen(t, c, "ready")
	waitForStatus(t, c, nativerunner.PidStatus{Status: nativerunner.ClaudeWaiting, WaitingFor: "permission prompt"}, 1)
	return c
}

func TestHolderSocket_StatusCarriesOSCState(t *testing.T) {
	c := startDenyHolder(t, `printf '\033]7501;state=working:app=claude\a'; cat >/dev/null`)
	if !c.Supports(nativerunner.OpTurnOSC) {
		t.Fatal("ping does not list the turn-osc marker")
	}
	// denyChild runs the tail after one key.
	if err := c.SendKeys("x"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "status carries the OSC state", func() bool {
		got, err := c.ProgramState()
		return err == nil && got == "working"
	})
	if _, turn, _ := c.Status(); turn < 2 {
		t.Errorf("turn = %d; want the pid status change and the OSC working both counted", turn)
	}
}

func TestHolderSocket_OlderHolderLacksTurnMarker(t *testing.T) {
	nativerunner.SetHolderOptionalOps(t, nil)
	_, c, _ := startHolder(t, echoChild)
	if c.Supports(nativerunner.OpTurnOSC) {
		t.Error("an older holder lists turn-osc")
	}
	waitForScreen(t, c, "ready")
	if _, _, err := c.Status(); err != nil {
		t.Errorf("status on an older holder: %v", err)
	}
}

func TestHolderSocket_DenyReason(t *testing.T) {
	c := startDenyHolder(t, denyGoesIdle)

	stop := make(chan struct{})
	sawIdle := make(chan bool, 1)
	go func() {
		idle := false
		defer func() { sawIdle <- idle }()
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
			}
			if s, _, err := c.Status(); err == nil && s.Status == nativerunner.ClaudeIdle {
				idle = true
			}
		}
	}()
	err := c.DenyWithReason("use rg")
	close(stop)
	if err != nil {
		t.Fatal(err)
	}
	if <-sawIdle {
		t.Error("status showed idle during the op")
	}
	waitForScreen(t, c, "got:use rg")
	if s, _, _ := c.Status(); s == nil || s.Status != nativerunner.ClaudeBusy {
		t.Errorf("status after the op = %+v; want busy", s)
	}
}

func TestHolderSocket_DenyReasonIdleTimeout(t *testing.T) {
	nativerunner.SetHolderDenyTimeout(t, 300*time.Millisecond)
	c := startDenyHolder(t, denyStaysWaiting)

	if err := c.DenyWithReason("use rg"); err == nil {
		t.Fatal("deny-reason succeeded with no idle")
	}
}

// idleThenExit goes idle, then exits on the next line.
const idleThenExit = `st idle; read -r l; exit 3`

func TestHolderSocket_StatusFailsOnceClaudeExits(t *testing.T) {
	// Hold the holder before its drain, while holder.sock still answers.
	release := make(chan struct{})
	nativerunner.SetHolderAfterExit(t, func() { <-release })
	c := startDenyHolder(t, idleThenExit)
	defer close(release)
	if err := c.SendKeys("x"); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, c, nativerunner.PidStatus{Status: nativerunner.ClaudeIdle}, 2)

	if err := c.SendKeys("quit\r"); err != nil {
		t.Fatal(err)
	}
	var err error
	waitFor(t, "status fails", func() bool {
		var s *nativerunner.PidStatus
		s, _, err = c.Status()
		if err == nil && s.Status != nativerunner.ClaudeIdle {
			t.Fatalf("status = %+v; want idle until it fails", s)
		}
		return err != nil
	})
	var exited *nativerunner.ClaudeExitedError
	if !errors.As(err, &exited) || exited.Code != 3 {
		t.Errorf("status error = %v; want ClaudeExitedError with code 3", err)
	}
}

func waitForScreen(t *testing.T, c *nativerunner.HolderClient, want string) {
	t.Helper()
	waitFor(t, "the screen shows "+want, func() bool {
		rows, _, err := c.Screen(10)
		return err == nil && strings.Contains(strings.Join(rows, "\n"), want)
	})
}

// shortTempDir keeps holder.sock under the unix socket path limit, which a
// t.TempDir path can exceed on macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gxh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// denySlowIdle goes idle only after a pause, so the deny-reason step lasts
// long enough to act in the middle of it. The second line is what the held
// attach input reads as.
const denySlowIdle = `sleep 0.3; st idle; read -r l; st busy; printf 'got:%s\n' "$l"; read -r m; printf 'then:%s\n' "$m"; cat >/dev/null`

func TestHolderSocket_DenyReasonLocksKeysAndHoldsAttachInput(t *testing.T) {
	c := startDenyHolder(t, denySlowIdle)
	a, err := c.Attach()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })

	denied := make(chan error, 1)
	go func() { denied <- c.DenyWithReason("use rg") }()
	// An empty keys op writes nothing, so it probes the lock without typing.
	waitFor(t, "keys refused during the step", func() bool {
		return errors.Is(c.SendKeys(""), nativerunner.ErrHolderDenyBusy)
	})
	if err := a.Paste("held"); err != nil {
		t.Fatal(err)
	}
	if err := a.SendKey(screen.Key{Code: '\r'}); err != nil {
		t.Fatal(err)
	}
	if err := c.SendKeys("q"); !errors.Is(err, nativerunner.ErrHolderDenyBusy) {
		t.Fatalf("keys during the step = %v; want ErrHolderDenyBusy", err)
	}
	if err := <-denied; err != nil {
		t.Fatal(err)
	}
	// The reason reaches the child first, then the held input in order.
	waitForScreen(t, c, "got:use rg")
	waitForScreen(t, c, "then:held")
	if err := c.SendKeys(""); err != nil {
		t.Fatalf("keys after the step = %v; want accepted", err)
	}
}
