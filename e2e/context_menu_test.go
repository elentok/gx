package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

const menuTitle = `Actions · Ticket "tiny"`

// click presses and releases button over cell (x, y), once the command
// reports the mouse.
func (v *vtTerm) click(x, y int, button vt.MouseButton) {
	v.t.Helper()
	v.waitFor("mouse reporting", 5*time.Second, func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.mouseTracking
	})
	v.mu.Lock()
	defer v.mu.Unlock()
	v.emu.SendMouse(vt.MouseClick{X: x, Y: y, Button: button})
	v.emu.SendMouse(vt.MouseRelease{X: x, Y: y, Button: button})
}

func (v *vtTerm) waitGone(text string, timeout time.Duration) {
	v.t.Helper()
	v.waitFor("no "+text, timeout, func() bool {
		_, _, ok := v.find(text)
		return !ok
	})
}

// waitMenuBelow waits for the ticket's Context menu and checks it opened
// right under the row at rowY, its left edge under the row's panel.
func (v *vtTerm) waitMenuBelow(rowY int) {
	v.t.Helper()
	x, y := v.waitText(menuTitle, 5*time.Second)
	// The title sits in the top border: "╭ Actions · …" from column 1.
	if y != rowY+1 || x != 3 {
		v.t.Fatalf("menu title at (%d,%d), want (3,%d) right under the row; screen:\n%s",
			x, y, rowY+1, strings.Join(v.screen(), "\n"))
	}
}

// waitStatus waits for the ticket file to carry status.
func waitStatus(t *testing.T, v *vtTerm, path, status string) {
	t.Helper()
	v.waitFor("status: "+status+" in "+filepath.Base(path), 10*time.Second, func() bool {
		data, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(data), "\nstatus: "+status+"\n")
	})
}

// TestContextMenu_OpensClosesAndRuns drives a ticket's Context menu in the
// real Tickets tab, by keyboard and by mouse: it opens under its row,
// closes on esc and on a click outside it, and runs the picked action.
func TestContextMenu_OpensClosesAndRuns(t *testing.T) {
	e := newAgentsEnv(t)
	ticket := filepath.Join(e.dataHome, "gx", "tickets", e.project, "some-epic", "issues", "01-tiny.md")
	v := startTUI(t, e, "tickets")
	v.waitText("some-epic", 30*time.Second)
	v.send("l") // expand the epic
	_, rowY := v.waitText("01 tiny", 5*time.Second)
	v.send("j")
	v.waitText("Status: open", 5*time.Second) // the ticket's preview

	t.Run("keyboard", func(t *testing.T) {
		v.send("m")
		v.waitMenuBelow(rowY)
		v.send("\x1b")
		v.waitGone(menuTitle, 5*time.Second)

		v.send("m")
		v.waitMenuBelow(rowY)
		v.send("draft")
		v.waitText("› draft", 5*time.Second)
		v.send("\r")
		v.waitGone(menuTitle, 5*time.Second)
		waitStatus(t, v, ticket, "draft")
	})

	t.Run("mouse", func(t *testing.T) {
		v.click(10, rowY, vt.MouseRight)
		v.waitMenuBelow(rowY)
		v.click(120, 35, vt.MouseLeft) // over the preview, outside the menu
		v.waitGone(menuTitle, 5*time.Second)

		v.click(10, rowY, vt.MouseRight)
		v.waitMenuBelow(rowY)
		x, y := v.waitText("Set open", 5*time.Second)
		v.click(x, y, vt.MouseLeft)
		v.waitGone(menuTitle, 5*time.Second)
		waitStatus(t, v, ticket, "open")
	})
}
