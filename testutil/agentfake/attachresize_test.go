package agentfake

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
)

// attachView is what one attach shows: its rows by index, rebuilt from the
// frames it gets.
type attachView struct {
	mu   sync.Mutex
	rows map[int]string
	// last is the latest frame, for its scroll fields.
	last nativerunner.Frame
	// copied gets the attach's copy replies.
	copied chan string
}

// viewResized skips a's first frame, which predates its size, resizes it and
// follows its frames from there.
func (p *heldProgram) viewResized(a *nativerunner.HolderAttach, cols, rows int) *attachView {
	p.t.Helper()
	if _, err := a.Next(); err != nil {
		p.t.Fatal(err)
	}
	if err := a.Resize(cols, rows); err != nil {
		p.t.Fatal(err)
	}
	v := &attachView{rows: map[int]string{}, copied: make(chan string, 1)}
	go func() {
		for {
			m, err := a.Next()
			if err != nil {
				return
			}
			switch m.Type {
			case nativerunner.StreamFrame:
				v.apply(m.Frame)
			case nativerunner.StreamCopy:
				v.copied <- m.Text
			}
		}
	}()
	return v
}

func (v *attachView) apply(f nativerunner.Frame) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, r := range f.Rows {
		v.rows[r.Index] = r.ANSI
	}
	v.last = f
}

// shows reports whether the view is exactly width×height and shows text.
func (v *attachView) shows(width, height int, text string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	found := false
	for i, r := range v.rows {
		if i >= height || ansi.StringWidth(r) != width {
			return false
		}
		found = found || strings.Contains(ansi.Strip(r), text)
	}
	return found && len(v.rows) == height
}

func (v *attachView) String() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	var b strings.Builder
	for i := range len(v.rows) + 1 {
		if r, ok := v.rows[i]; ok {
			fmt.Fprintf(&b, "%d(w%d): %q\n", i, ansi.StringWidth(r), ansi.Strip(r))
		}
	}
	return b.String()
}

func (p *heldProgram) waitView(v *attachView, width, height int, text string) {
	p.t.Helper()
	p.waitFor(fmt.Sprintf("a %dx%d view showing %s", width, height, text), func() bool { return v.shows(width, height, text) })
}

// waitSize makes the program clear the screen, draw a line wider than any
// view here, and report its terminal size, until it reports want. It asks
// again because a resize or key sent just before can reach the holder after
// the program answered.
func (p *heldProgram) waitSize(want string) {
	p.t.Helper()
	var asked time.Time
	p.waitFor("the screen shows size="+want, func() bool {
		if time.Since(asked) > 200*time.Millisecond {
			p.control(Control{Action: ActionWrite, Output: "\x1b[2J\x1b[H" + strings.Repeat("-", 99) + "\r\n"})
			p.control(Control{Action: ActionSize})
			asked = time.Now()
		}
		return strings.Contains(strings.Join(p.rows(), "\n"), "size="+want)
	})
}

func TestAttachResize_LastViewerToFocusOrTypeOwnsTheSize(t *testing.T) {
	p, a := holdProgramAttach(t)
	va := p.viewResized(a, 100, 30)
	p.waitSize("100x30")

	b, err := p.client.Attach()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	vb := p.viewResized(b, 60, 20)
	p.waitSize("60x20")
	// a's rows past the pty's 20 and columns past its 60 are padding.
	p.waitView(va, 100, 30, "size=60x20")

	if err := a.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}
	p.waitSize("100x30")
	// The 99-dash line and rows past b's 20 are cropped.
	p.waitView(vb, 60, 20, "size=100x30")

	_ = a.Close()
	_ = b.Close()
	p.waitSize("100x30")
}
