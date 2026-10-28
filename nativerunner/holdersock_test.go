package nativerunner_test

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/syshealth"
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

// pidChild writes a starting pid status file, then sets each status line it
// reads, adding a waitingFor only while waiting, as claude does.
const pidChild = `d="$HOME/.claude/sessions"; mkdir -p "$d"; f="$d/$$.json"; t=$(date +%s)000
printf '{"pid":%s,"startedAt":%s}' $$ "$t" > "$f"; printf 'ready\n'
while read -r s; do
  w=""; [ "$s" = waiting ] && w=',"waitingFor":"input needed"'
  printf '{"pid":%s,"startedAt":%s,"status":"%s"%s}' $$ "$t" "$s" "$w" > "$f.tmp" && mv "$f.tmp" "$f"
  printf 'set:%s\n' "$s"
done`

func TestHolderSocket_StatusCountsTurns(t *testing.T) {
	t.Setenv("HOME", shortTempDir(t))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	_, c, _ := startHolder(t, pidChild)

	waitForStatus(t, c, nativerunner.PidStatus{Status: nativerunner.ClaudeStarting}, 0)
	steps := []nativerunner.PidStatus{
		{Status: nativerunner.ClaudeBusy},
		{Status: nativerunner.ClaudeWaiting, WaitingFor: "input needed"},
		{Status: nativerunner.ClaudeBusy},
		{Status: nativerunner.ClaudeIdle},
	}
	for i, want := range steps {
		if err := c.SendKeys(want.Status + "\r"); err != nil {
			t.Fatal(err)
		}
		waitForStatus(t, c, want, i+1)
	}
}

func waitForStatus(t *testing.T, c *nativerunner.HolderClient, want nativerunner.PidStatus, turn int) {
	t.Helper()
	waitFor(t, fmt.Sprintf("status %+v at turn %d", want, turn), func() bool {
		got, n, err := c.Status()
		return err == nil && got != nil && *got == want && n == turn
	})
}

func TestRunHolder_LongSocketPathNamesTheCap(t *testing.T) {
	dir := filepath.Join(shortTempDir(t), strings.Repeat("d", 120))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := nativerunner.RunHolder(dir, []string{"true"})
	if err == nil || !strings.Contains(err.Error(), "unix socket path") {
		t.Fatalf("err = %v, want one naming the unix socket path length cap", err)
	}
}

func TestHolderSocket_PingNamesPidsAndStartTimes(t *testing.T) {
	_, c, _ := startHolder(t, echoChild)

	if c.HolderPID() != os.Getpid() {
		t.Errorf("holder pid = %d; want %d", c.HolderPID(), os.Getpid())
	}
	if c.ChildPID() <= 0 || c.ChildPID() == c.HolderPID() {
		t.Fatalf("child pid = %d; want a pid apart from the holder's", c.ChildPID())
	}
	for _, tc := range []struct {
		name string
		pid  int
		got  func() (int64, bool)
	}{
		{"holder", c.HolderPID(), c.HolderStart},
		{"child", c.ChildPID(), c.ChildStart},
	} {
		want, err := syshealth.ProcessStart(tc.pid)
		if err != nil {
			t.Fatalf("%s start from syshealth: %v", tc.name, err)
		}
		got, ok := tc.got()
		if !ok {
			t.Errorf("%s start not known", tc.name)
		} else if got < want-1 || got > want+1 {
			t.Errorf("%s start = %d; want %d within 1s", tc.name, got, want)
		}
	}
}

func TestDialHolder_OldHolderStartTimesNotKnown(t *testing.T) {
	dir := shortTempDir(t)
	ln, err := net.Listen("unix", filepath.Join(dir, nativerunner.HolderSocketFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte(`{"version":1,"ops":["deny-reason"],"pid":4242}` + "\n"))
	}()

	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.ChildPID() != 4242 {
		t.Errorf("child pid = %d; want 4242", c.ChildPID())
	}
	if c.HolderPID() != 0 {
		t.Errorf("holder pid = %d; want 0 (not known)", c.HolderPID())
	}
	if _, ok := c.HolderStart(); ok {
		t.Error("holder start reported known for an old holder")
	}
	if _, ok := c.ChildStart(); ok {
		t.Error("child start reported known for an old holder")
	}
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
	return startDenyHolderWith(t, tail, func(t *testing.T) { t.Setenv("HOME", shortTempDir(t)) })
}

// startDenyHolderWith lets setup choose the HOME the holder runs with.
func startDenyHolderWith(t *testing.T, tail string, setup func(*testing.T)) *nativerunner.HolderClient {
	t.Helper()
	setup(t)
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

// vimConfigHome turns vim mode on in the HOME startDenyHolder points at.
func vimConfigHome(t *testing.T) {
	t.Helper()
	home := shortTempDir(t)
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"editorMode":"vim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

// denyVimNormal shows the input box with no INSERT marker once idle, and
// shows the marker after a single `i`.
const denyVimNormal = `stty -icanon -echo min 1; printf '\n❯ \n'; st idle; dd bs=1 count=1 2>/dev/null >/dev/null; printf -- '-- INSERT --\n'; stty icanon; read -r l; st busy; printf 'got:%s\n' "$l"; cat >/dev/null`

func TestHolderSocket_DenyReason_VimNormalGetsIBeforeTheReason(t *testing.T) {
	c := startDenyHolderWith(t, denyVimNormal, vimConfigHome)
	if err := c.DenyWithReason("use rg"); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, c, "got:use rg")
}

func TestHolderSocket_DenyReason_VimGiveUpFailsTheDenyWithoutTyping(t *testing.T) {
	c := startDenyHolderWith(t, `printf '\n❯ \n'; st idle; read -r l; printf 'got:%s\n' "$l"; cat >/dev/null`, vimConfigHome)
	if err := c.DenyWithReason("use rg"); err == nil {
		t.Fatal("deny-reason succeeded with no INSERT marker")
	}
	screen, _, _ := c.Screen(0)
	if strings.Contains(strings.Join(screen, "\n"), "got:use rg") {
		t.Errorf("reason was typed into NORMAL mode: %q", screen)
	}
}

// idleThenExit goes idle, then exits on the next line.
const idleThenExit = `st idle; read -r l; exit 3`

func TestHolderSocket_StatusFailsOnceClaudeExits(t *testing.T) {
	// Hold the holder before its wind-down, while holder.sock still answers.
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

// pasteChild turns bracketed paste on, then shows each line it reads with
// control bytes visible.
const pasteChild = `printf '\033[?2004hready\n'; while IFS= read -r l; do printf '%s\n' "$l" | cat -v; done`

func TestHolderSocket_PasteIsBracketed(t *testing.T) {
	_, c, _ := startHolder(t, pasteChild)
	if !c.Supports(nativerunner.OpPaste) {
		t.Fatal("ping does not list paste")
	}
	waitForScreen(t, c, "ready")
	if err := c.Paste("hi"); err != nil {
		t.Fatal(err)
	}
	if err := c.SendKeys("\r"); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, c, "^[[200~hi^[[201~")
}

// countChild turns bracketed paste on and counts the bytes of the first n it
// reads in raw mode, so a tty line limit can't cut a long input.
const countChild = `stty raw -echo; printf '\033[?2004hready\r\n'; n=$(head -c %d | wc -c | tr -d ' '); stty sane; printf 'count:%%s\n' "$n"; cat >/dev/null`

func TestHolderClient_TypeTextPastesLongText(t *testing.T) {
	long := strings.Repeat("x", 1500)
	pasted := len(long) + len("\x1b[200~\x1b[201~")
	_, c, _ := startHolder(t, fmt.Sprintf(countChild, pasted))
	waitForScreen(t, c, "ready")
	if err := c.TypeText(long); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, c, fmt.Sprintf("count:%d", pasted))
}

func TestHolderClient_TypeTextSendsShortTextAsKeys(t *testing.T) {
	_, c, _ := startHolder(t, pasteChild)
	waitForScreen(t, c, "ready")
	if err := c.TypeText("hi\r"); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, c, "hi")
	rows, _, err := c.Screen(10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(rows, "\n"), "200~") {
		t.Errorf("short text went as a paste: %q", rows)
	}
}

func TestHolderClient_TypeTextWithoutPasteOpSendsKeys(t *testing.T) {
	nativerunner.SetHolderOptionalOps(t, nil)
	_, c, _ := startHolder(t, echoChild)
	waitForScreen(t, c, "ready")
	long := strings.Repeat("y", 600)
	if err := c.TypeText(long + "\r"); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, c, "got:yyyy")
}

// dimChild prints "dimmed" with the SGR dim attribute on, then idles.
const dimChild = `printf '\033[2mdimmed\033[0m\n'; cat >/dev/null`

func TestHolderSocket_StyledScreen(t *testing.T) {
	_, c, _ := startHolder(t, dimChild)
	if !c.Supports(nativerunner.OpScreenStyled) {
		t.Fatal("ping does not list screen-styled")
	}
	waitForScreen(t, c, "dimmed")

	rows, err := c.StyledScreen()
	if err != nil {
		t.Fatal(err)
	}
	styled := strings.Join(rows, "\n")
	if !strings.Contains(styled, "\x1b[") || !strings.Contains(styled, "2") || !strings.Contains(styled, "dimmed") {
		t.Errorf("styled screen has no SGR dim span: %q", styled)
	}
	plain, _, err := c.Screen(0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(plain, "\n"), "\x1b") {
		t.Errorf("plain screen carries escape codes: %q", plain)
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

func TestHolderSocket_SecondDenyReasonDuringStepIsBusy(t *testing.T) {
	c := startDenyHolder(t, denySlowIdle)
	denied := make(chan error, 1)
	go func() { denied <- c.DenyWithReason("use rg") }()
	waitFor(t, "keys refused during the step", func() bool {
		return errors.Is(c.SendKeys(""), nativerunner.ErrHolderDenyBusy)
	})
	if err := c.DenyWithReason("other"); !errors.Is(err, nativerunner.ErrHolderDenyBusy) {
		t.Fatalf("second deny-reason = %v; want ErrHolderDenyBusy", err)
	}
	if err := <-denied; err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, c, "got:use rg")
}
