package agentfake

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/nativerunner"
)

// writeLines makes the program write lines "L<from>   x" up to, not
// including, to, and waits for the screen to show the last.
func (p *heldProgram) writeLines(from, to int) {
	p.t.Helper()
	var b strings.Builder
	for i := from; i < to; i++ {
		fmt.Fprintf(&b, "L%03d   x\r\n", i)
	}
	p.control(Control{Action: ActionWrite, Output: b.String()})
	p.waitScreen(fmt.Sprintf("L%03d", to-1))
}

// row returns the view's row i without styles or padding.
func (v *attachView) row(i int) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return strings.TrimRight(ansi.Strip(v.rows[i]), " ")
}

func (v *attachView) frame() nativerunner.Frame {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.last
}

func TestAttachScroll_ViewsScrollbackPerViewerAndStaysPutWhileOutputArrives(t *testing.T) {
	p, a := holdProgramAttach(t)
	va := p.viewResized(a, 40, 10)
	p.waitSize("40x10")
	b, err := p.client.Attach()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	vb := p.viewResized(b, 40, 10)

	// The screen shows L091-L099 above the cursor's empty row.
	p.writeLines(0, 100)
	if err := a.Scroll(5); err != nil {
		t.Fatal(err)
	}
	p.waitFor("a scrolled view starting at L086", func() bool {
		f := va.frame()
		return va.row(0) == "L086   x" && va.row(9) == "L095   x" && f.ScrollOffset == 5 && f.ScrollMax > 86
	})

	p.writeLines(100, 103)
	p.waitFor("a's offset and new-lines count grown by 3", func() bool {
		f := va.frame()
		return f.ScrollOffset == 8 && f.NewLines == 3
	})
	if va.row(0) != "L086   x" {
		t.Errorf("a's view moved while scrolled:\n%s", va)
	}
	p.waitView(vb, 40, 10, "L102")
	if f := vb.frame(); f.ScrollOffset != 0 || f.NewLines != 0 {
		t.Errorf("b's frame = offset %d, new %d; a's scroll moved b", f.ScrollOffset, f.NewLines)
	}

	// Row 7 is the scrollback's last line, L093; row 8 the screen's first.
	f := va.frame()
	top := f.ScrollMax - f.ScrollOffset
	if err := a.Copy(nativerunner.LinePos{Line: top + 7, Col: 2}, nativerunner.LinePos{Line: top + 8, Col: 7}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-va.copied:
		if want := "93   x\nL094"; got != want {
			t.Errorf("copy = %q, want %q", got, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no copy reply")
	}

	if err := a.Scroll(0); err != nil {
		t.Fatal(err)
	}
	p.waitFor("a back at the live bottom", func() bool {
		f := va.frame()
		return f.ScrollOffset == 0 && f.NewLines == 0 && va.row(8) == "L102   x"
	})
}
