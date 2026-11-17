// Package vtterm runs a command in a pty and keeps its screen in a terminal
// emulator, which answers the command's terminal queries as a real terminal
// would. It reads no environment: the caller sets cmd.Env.
package vtterm

import (
	"image/color"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// Term is a command running in a pty, with its screen kept in an emulator.
type Term struct {
	t    *testing.T
	ptmx interface{ WriteString(string) (int, error) }

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

// Start runs cmd in a cols x rows pty. A cleanup kills the process.
func Start(t *testing.T, cmd *exec.Cmd, cols, rows int) *Term {
	t.Helper()
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		t.Fatal(err)
	}
	v := &Term{t: t, ptmx: ptmx, emu: vt.NewEmulator(cols, rows), cursorVisible: true}
	v.emu.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) {
		v.cursorVisible = visible
	}})
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
func (v *Term) trackMouseModes(out []byte) {
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

// WheelUp turns the wheel up over cell (x, y) as a terminal in the alternate
// screen does: a mouse event when the command reports the mouse, else (the
// alternate scroll mode) arrow keys.
func (v *Term) WheelUp(x, y int) {
	v.t.Helper()
	v.mu.Lock()
	tracking := v.mouseTracking
	if tracking {
		v.emu.SendMouse(vt.MouseWheel{X: x, Y: y, Button: vt.MouseWheelUp})
	}
	v.mu.Unlock()
	if !tracking {
		v.Send(strings.Repeat("\x1b[A", 3))
	}
}

// Click presses and releases button over cell (x, y), once the command
// reports the mouse.
func (v *Term) Click(x, y int, button vt.MouseButton) {
	v.t.Helper()
	v.WaitFor("mouse reporting", 5*time.Second, func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.mouseTracking
	})
	v.mu.Lock()
	defer v.mu.Unlock()
	v.emu.SendMouse(vt.MouseClick{X: x, Y: y, Button: button})
	v.emu.SendMouse(vt.MouseRelease{X: x, Y: y, Button: button})
}

// Send types s into the command.
func (v *Term) Send(s string) {
	v.t.Helper()
	if _, err := v.ptmx.WriteString(s); err != nil {
		v.t.Fatal(err)
	}
}

// Screen is the emulator's text, one entry per row.
func (v *Term) Screen() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return strings.Split(v.emu.String(), "\n")
}

// Find returns the cell where text starts on screen.
func (v *Term) Find(text string) (x, y int, ok bool) {
	for y, row := range v.Screen() {
		if before, _, found := strings.Cut(row, text); found {
			return ansi.StringWidth(before), y, true
		}
	}
	return 0, 0, false
}

// WaitFor polls until cond holds, failing with the screen after timeout.
func (v *Term) WaitFor(what string, timeout time.Duration, cond func() bool) {
	v.t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if cond() {
			return
		}
	}
	v.t.Fatalf("timed out waiting for %s; screen:\n%s", what, strings.Join(v.Screen(), "\n"))
}

// WaitText waits for text on screen and returns where it starts.
func (v *Term) WaitText(text string, timeout time.Duration) (x, y int) {
	v.t.Helper()
	v.WaitFor("text "+text, timeout, func() bool {
		var ok bool
		x, y, ok = v.Find(text)
		return ok
	})
	return x, y
}

// WaitGone waits for text to leave the screen.
func (v *Term) WaitGone(text string, timeout time.Duration) {
	v.t.Helper()
	v.WaitFor("no "+text, timeout, func() bool {
		_, _, ok := v.Find(text)
		return !ok
	})
}

// FG is the foreground colour of cell (x, y), nil when unset.
func (v *Term) FG(x, y int) color.Color {
	v.mu.Lock()
	defer v.mu.Unlock()
	if c := v.emu.CellAt(x, y); c != nil {
		return c.Style.Fg
	}
	return nil
}

// Cursor is the cursor cell and whether it is shown.
func (v *Term) Cursor() (x, y int, visible bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p := v.emu.CursorPosition()
	return p.X, p.Y, v.cursorVisible
}
