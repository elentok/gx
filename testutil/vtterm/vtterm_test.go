package vtterm_test

import (
	"image/color"
	"os/exec"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/testutil/vtterm"
)

func TestTerm_ShowsTextCursorAndColour(t *testing.T) {
	cmd := exec.Command("sh", "-c", `printf '\033[2;3H\033[31mred\033[0m'; sleep 30`)
	v := vtterm.Start(t, cmd, 20, 5)

	x, y := v.WaitText("red", 5*time.Second)
	if x != 2 || y != 1 {
		t.Fatalf("red at (%d,%d), want (2,1)", x, y)
	}
	if cx, cy, visible := v.Cursor(); cx != 5 || cy != 1 || !visible {
		t.Fatalf("cursor (%d,%d) visible=%v, want (5,1) visible", cx, cy, visible)
	}
	if got := v.FG(x, y); got != color.Color(ansi.Red) {
		t.Fatalf("red cell has colour %v, want %v", got, ansi.Red)
	}
	if plain := v.FG(0, 0); plain != nil {
		t.Fatalf("unstyled cell has colour %v", plain)
	}
}
