package e2e

import (
	"encoding/json"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

var (
	buildTUIFakeOnce sync.Once
	tuiFakeBinDir    string
	buildTUIFakeErr  error
)

// tuiFakeBinary builds testutil/agentfake's fake interactive claude as the
// literal binary name "claude" and returns its directory.
func tuiFakeBinary(t *testing.T) string {
	t.Helper()
	buildTUIFakeOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gx-e2e-agentfaketui")
		if err != nil {
			buildTUIFakeErr = err
			return
		}
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "claude"), "github.com/elentok/gx/testutil/agentfake/cmd/agentfaketui")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildTUIFakeErr = err
			t.Logf("go build output:\n%s", out)
			return
		}
		tuiFakeBinDir = dir
	})
	if buildTUIFakeErr != nil {
		t.Fatalf("build agentfaketui binary: %v", buildTUIFakeErr)
	}
	return tuiFakeBinDir
}

// agentsEnv is a real gx server of its own (its own state dir, so its own
// socket) whose claude is the TUI fake, and a project to run sessions in.
type agentsEnv struct {
	t       *testing.T
	bin     string
	repo    string
	project string
	// env isolates every gx process; serverEnv is the server's whole
	// environment, without TERM, as launchd starts it.
	env, serverEnv []string
}

func newAgentsEnv(t *testing.T) *agentsEnv {
	t.Helper()
	root, err := os.MkdirTemp("", "gxa")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	e := &agentsEnv{t: t, bin: gxBinary(t), repo: testutil.TempRepo(t), project: "agentsproj"}
	e.env = []string{
		"XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"HOME=" + filepath.Join(root, "home"),
		"PATH=" + tuiFakeBinary(t) + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); name != "TERM" && name != "COLORTERM" {
			e.serverEnv = append(e.serverEnv, kv)
		}
	}
	e.serverEnv = append(e.serverEnv, e.env...)

	store := filepath.Join(root, "data", "gx", "tickets")
	servertest.WriteTicket(t, store, e.project, "some-epic", "01", "tiny", "")
	servertest.SetProjectRepo(t, store, e.project, e.repo)

	e.run(e.serverEnv, "server", "start")
	t.Cleanup(func() {
		if t.Failed() {
			log, _ := os.ReadFile(filepath.Join(root, "state", "gx", "server.log"))
			t.Logf("server.log:\n%s", log)
		}
		e.run(e.serverEnv, "server", "stop")
	})
	return e
}

func (e *agentsEnv) run(env []string, args ...string) string {
	e.t.Helper()
	cmd := exec.Command(e.bin, args...)
	cmd.Dir = e.repo
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("gx %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// createSession starts a claude session in the repo and stops it at cleanup.
func (e *agentsEnv) createSession() {
	e.t.Helper()
	var res struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	out := e.run(e.serverEnv, "agents", "create", "--project", e.project, "--dir", e.repo, "--json")
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Session.ID == "" {
		e.t.Fatalf("agents create: %v\n%s", err, out)
	}
	e.t.Cleanup(func() {
		cmd := exec.Command(e.bin, "agents", "stop", res.Session.ID)
		cmd.Env = e.serverEnv
		_ = cmd.Run()
	})
}

// vtTerm runs a command in a pty and keeps its screen in a terminal emulator,
// which answers the command's terminal queries as a real terminal would.
type vtTerm struct {
	t    *testing.T
	ptmx *os.File

	mu            sync.Mutex
	emu           *vt.Emulator
	cursorVisible bool
	// mouseTracking is whether the command turned on mouse reporting; tail
	// keeps a mode sequence split across reads whole.
	mouseTracking bool
	tail          []byte
}

// mouseModeSeq is a DEC private mode set or reset.
var mouseModeSeq = regexp.MustCompile(`\x1b\[\?([0-9;]+)([hl])`)

func startVTTerm(t *testing.T, cmd *exec.Cmd, cols, rows int) *vtTerm {
	t.Helper()
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		t.Fatal(err)
	}
	v := &vtTerm{t: t, ptmx: ptmx, emu: vt.NewEmulator(cols, rows), cursorVisible: true}
	v.emu.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) { v.cursorVisible = visible }})
	go func() { _, _ = io.Copy(ptmx, v.emu) }()
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				v.mu.Lock()
				_, _ = v.emu.Write(buf[:n])
				v.trackMouseModes(buf[:n])
				v.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = ptmx.Close()
	})
	return v
}

// trackMouseModes follows the mouse reporting modes in the command's output,
// which the emulator keeps to itself.
func (v *vtTerm) trackMouseModes(out []byte) {
	data := append(v.tail, out...)
	for _, m := range mouseModeSeq.FindAllSubmatch(data, -1) {
		for mode := range strings.SplitSeq(string(m[1]), ";") {
			if mode == "1000" || mode == "1002" || mode == "1003" {
				v.mouseTracking = string(m[2]) == "h"
			}
		}
	}
	v.tail = append([]byte(nil), data[max(0, len(data)-16):]...)
}

// wheelUp turns the wheel up over cell (x, y) as a terminal in the alternate
// screen does: a mouse event when the command reports the mouse, else (the
// alternate scroll mode) arrow keys.
func (v *vtTerm) wheelUp(x, y int) {
	v.t.Helper()
	v.mu.Lock()
	tracking := v.mouseTracking
	if tracking {
		v.emu.SendMouse(vt.MouseWheel{X: x, Y: y, Button: vt.MouseWheelUp})
	}
	v.mu.Unlock()
	if !tracking {
		v.send(strings.Repeat("\x1b[A", 3))
	}
}

func (v *vtTerm) send(s string) {
	v.t.Helper()
	if _, err := v.ptmx.WriteString(s); err != nil {
		v.t.Fatal(err)
	}
}

// screen is the emulator's text, one entry per row.
func (v *vtTerm) screen() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return strings.Split(v.emu.String(), "\n")
}

// find returns the cell where text starts on screen.
func (v *vtTerm) find(text string) (x, y int, ok bool) {
	for y, row := range v.screen() {
		if before, _, found := strings.Cut(row, text); found {
			return ansi.StringWidth(before), y, true
		}
	}
	return 0, 0, false
}

// waitFor polls until cond holds, failing with the screen after timeout.
func (v *vtTerm) waitFor(what string, timeout time.Duration, cond func() bool) {
	v.t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if cond() {
			return
		}
	}
	v.t.Fatalf("timed out waiting for %s; screen:\n%s", what, strings.Join(v.screen(), "\n"))
}

func (v *vtTerm) waitText(text string, timeout time.Duration) (x, y int) {
	v.t.Helper()
	v.waitFor("text "+text, timeout, func() bool {
		var ok bool
		x, y, ok = v.find(text)
		return ok
	})
	return x, y
}

func (v *vtTerm) fg(x, y int) color.Color {
	v.mu.Lock()
	defer v.mu.Unlock()
	if c := v.emu.CellAt(x, y); c != nil {
		return c.Style.Fg
	}
	return nil
}

func (v *vtTerm) cursor() (x, y int, visible bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p := v.emu.CursorPosition()
	return p.X, p.Y, v.cursorVisible
}

// openAgentsTab runs `gx agents` against e's server in a pty and waits for
// the session's screen.
func openAgentsTab(t *testing.T, e *agentsEnv) *vtTerm {
	t.Helper()
	cmd := exec.Command(e.bin, "agents")
	cmd.Dir = e.repo
	cmd.Env = append(os.Environ(), append(e.env, "TERM=xterm-256color")...)
	v := startVTTerm(t, cmd, 140, 40)
	v.waitText("Welcome to Claude Code", 30*time.Second)
	return v
}

// TestAgentsTerminal_ShowsTheAgentsColors: a launchd server has no TERM, and
// claude, like any program, only colors its output on a terminal that says it
// supports color.
func TestAgentsTerminal_ShowsTheAgentsColors(t *testing.T) {
	e := newAgentsEnv(t)
	e.createSession()
	v := openAgentsTab(t, e)

	x, y := v.waitText("Welcome to Claude Code", 5*time.Second)
	v.waitFor("the welcome line in red", 5*time.Second, func() bool {
		return v.fg(x, y) == ansi.Red
	})
}

// TestAgentsTerminal_TypesShiftedLetters: Shift+h reaches the TUI as h with
// Shift, and must reach the agent as H.
func TestAgentsTerminal_TypesShiftedLetters(t *testing.T) {
	e := newAgentsEnv(t)
	e.createSession()
	v := openAgentsTab(t, e)

	v.send("\r") // focus the terminal
	time.Sleep(300 * time.Millisecond)
	v.send("Hi There!")
	v.waitText("> Hi There!", 10*time.Second)
}

// TestAgentsTerminal_ShowsTheAgentsCursor: the focused terminal shows the
// cursor where the agent left it.
func TestAgentsTerminal_ShowsTheAgentsCursor(t *testing.T) {
	e := newAgentsEnv(t)
	e.createSession()
	v := openAgentsTab(t, e)

	v.send("\r") // focus the terminal
	time.Sleep(300 * time.Millisecond)
	v.send("abc")
	px, py := v.waitText("> abc", 10*time.Second)
	v.waitFor("the cursor after the typed text", 5*time.Second, func() bool {
		x, y, visible := v.cursor()
		return visible && x == px+len("> abc") && y == py
	})
}

// TestAgentsTerminal_WheelNeverReachesTheAgentAsArrows: the wheel over the
// focused terminal scrolls gx's view of it. The terminal must not turn it
// into arrow keys for the agent, which claude warns about.
func TestAgentsTerminal_WheelNeverReachesTheAgentAsArrows(t *testing.T) {
	e := newAgentsEnv(t)
	e.createSession()
	v := openAgentsTab(t, e)

	v.send("\r") // focus the terminal
	time.Sleep(300 * time.Millisecond)
	x, y := v.waitText("Welcome to Claude Code", 5*time.Second)
	v.wheelUp(x, y)
	time.Sleep(300 * time.Millisecond)
	// Typing snaps the view back to the live screen, where the fake echoes
	// all it read: arrow keys would show before the x.
	v.send("x")
	v.waitText("> x", 10*time.Second)
}
