package nativerunner_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
)

// attachTestHolder runs a holder whose child prints two rows, rewrites the
// second once the change file appears, and exits with 3 once the stop file
// appears. It returns once the two rows are on the screen.
func attachTestHolder(t *testing.T) (c *nativerunner.HolderClient, change, stop string, finished chan struct{}) {
	t.Helper()
	dir := shortTempDir(t)
	change, stop = filepath.Join(dir, "change"), filepath.Join(dir, "stop")
	child := `printf 'first\nsecond'; while [ ! -e "$2" ]; do ` +
		`if [ -e "$1" ] && [ -z "$changed" ]; then printf '\033[2;1H\033[31mchanged\033[0m'; changed=1; fi; ` +
		`sleep 0.05; done; exit 3`
	c, finished = startTestHolder(t, dir, settableHarness{w: &settableWatcher{}}, []string{"sh", "-c", child, "sh", change, stop})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	waitFor(t, "child output on screen", func() bool {
		rows, _, err := c.Screen(0)
		return err == nil && slices.Contains(rows, "second")
	})
	return c, change, stop, finished
}

func attach(t *testing.T, c *nativerunner.HolderClient) *nativerunner.HolderAttach {
	t.Helper()
	a, err := c.Attach()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func nextAttach(t *testing.T, a *nativerunner.HolderAttach) nativerunner.AttachMessage {
	t.Helper()
	m, err := a.Next()
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	return m
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func rowIndexes(f nativerunner.Frame) []int {
	var idx []int
	for _, r := range f.Rows {
		idx = append(idx, r.Index)
	}
	return idx
}

func TestHolderAttach_FullFrameThenDirtyRowsThenExit(t *testing.T) {
	c, change, stop, _ := attachTestHolder(t)
	if !c.Supports(nativerunner.OpAttach) {
		t.Fatal("ping does not list attach")
	}
	attaches := []*nativerunner.HolderAttach{attach(t, c), attach(t, c)}

	for i, a := range attaches {
		m := nextAttach(t, a)
		if m.Type != nativerunner.StreamFrame || !m.Frame.Full || len(m.Frame.Rows) != nativerunner.HolderRows {
			t.Fatalf("attach %d first message = type %d with %d rows; want a full frame of %d rows", i, m.Type, len(m.Frame.Rows), nativerunner.HolderRows)
		}
		if !strings.Contains(m.Frame.Rows[0].ANSI, "first") || !strings.Contains(m.Frame.Rows[1].ANSI, "second") {
			t.Errorf("attach %d full frame starts %q, %q; want the child's rows", i, m.Frame.Rows[0].ANSI, m.Frame.Rows[1].ANSI)
		}
		if m.Frame.Cursor != (nativerunner.FrameCursor{X: 6, Y: 1, Visible: true}) {
			t.Errorf("attach %d cursor = %+v; want after \"second\"", i, m.Frame.Cursor)
		}
	}

	touch(t, change)
	for i, a := range attaches {
		m := nextAttach(t, a)
		if m.Type != nativerunner.StreamFrame || m.Frame.Full || !slices.Equal(rowIndexes(m.Frame), []int{1}) {
			t.Fatalf("attach %d after the change = type %d rows %v; want a frame of row 1 only", i, m.Type, rowIndexes(m.Frame))
		}
		if got := m.Frame.Rows[0].ANSI; !strings.Contains(got, "\x1b[31mchanged") {
			t.Errorf("attach %d row 1 = %q; want it styled red", i, got)
		}
	}

	touch(t, stop)
	for i, a := range attaches {
		if m := nextAttach(t, a); m.Type != nativerunner.StreamExit || m.ExitCode != 3 {
			t.Errorf("attach %d on exit = %+v; want exit code 3", i, m)
		}
	}
}

// scriptHolder runs a holder whose child runs script with the holder's dir as
// $1, and returns once the child printed "ready".
func scriptHolder(t *testing.T, script string) (c *nativerunner.HolderClient, dir string) {
	t.Helper()
	dir = shortTempDir(t)
	c, finished := startTestHolder(t, dir, settableHarness{w: &settableWatcher{}}, []string{"sh", "-c", script, "sh", dir})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	waitFor(t, "child ready", func() bool {
		rows, _, err := c.Screen(0)
		return err == nil && strings.Contains(strings.Join(rows, "\n"), "ready")
	})
	return c, dir
}

func frameHas(f nativerunner.Frame, text string) bool {
	return slices.ContainsFunc(f.Rows, func(r nativerunner.FrameRow) bool { return strings.Contains(r.ANSI, text) })
}

// waitGo makes a child wait for the test to touch $1/go.
const waitGo = `while [ ! -e "$1/go" ]; do sleep 0.01; done; `

func TestHolderAttach_BurstIsPacedThenStopsItsTimers(t *testing.T) {
	c, dir := scriptHolder(t, `printf ready; `+waitGo+
		`i=0; while [ $i -lt 100 ]; do printf '\r%d ' $i; sleep 0.002; i=$((i+1)); done; printf '\rdone '; exec sleep 60`)
	a := attach(t, c)
	nextAttach(t, a)
	touch(t, filepath.Join(dir, "go"))

	var first, last time.Time
	frames := 0
	for {
		m := nextAttach(t, a)
		last = time.Now()
		if frames == 0 {
			first = last
		}
		frames++
		if frameHas(m.Frame, "done") {
			break
		}
	}
	span := last.Sub(first)
	if span < 5*nativerunner.FrameInterval {
		t.Fatalf("burst spanned %v; too short to show pacing", span)
	}
	if limit := int(span/nativerunner.FrameInterval) + 2; frames > limit {
		t.Errorf("%d frames over %v; want at most %d", frames, span, limit)
	}

	waitFor(t, "pacing timers stopped", func() bool { return nativerunner.ArmedFrameTimers() == 0 })
}

func TestHolderAttach_KeyEchoSkipsThePacing(t *testing.T) {
	c, _ := scriptHolder(t, `printf ready; exec cat`)
	a := attach(t, c)
	nextAttach(t, a)

	const keys = 10
	start := time.Now()
	for i := range keys {
		if err := a.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
			t.Fatal(err)
		}
		for echo := "ready" + strings.Repeat("x", i+1); !frameHas(nextAttach(t, a).Frame, echo); {
		}
	}
	// Paced at one frame per interval, each echo would wait out the interval
	// since the frame before it.
	if took := time.Since(start); took >= keys*nativerunner.FrameInterval {
		t.Errorf("%d key echoes took %v; want each sooner than the frame interval", keys, took)
	}
}

func TestHolderAttach_SlowClientRecoversWithAFullFrame(t *testing.T) {
	// Redraws every row but the first, so only a full frame carries them all.
	c, dir := scriptHolder(t, `printf ready; `+waitGo+
		`i=0; while true; do line=$(printf '%0190d' $i); r=2; `+
		`while [ $r -le 50 ]; do printf '\033[%d;1H%s' $r "$line"; r=$((r+1)); done; i=$((i+1)); sleep 0.005; done`)
	slow, steady := attach(t, c), attach(t, c)
	nextAttach(t, slow)
	nextAttach(t, steady)
	touch(t, filepath.Join(dir, "go"))
	if m := nextAttach(t, slow); len(m.Frame.Rows) == nativerunner.HolderRows {
		t.Fatalf("first frame of the redraw has all %d rows; want only the redrawn ones", len(m.Frame.Rows))
	}

	// slow stops reading long enough for its socket to fill; steady keeps up.
	steadyFrames := 0
	for start := time.Now(); time.Since(start) < 2*time.Second; steadyFrames++ {
		nextAttach(t, steady)
	}
	if steadyFrames < 10 {
		t.Errorf("steady attach got %d frames while the other stalled; want the holder unblocked", steadyFrames)
	}

	for i := 0; len(nextAttach(t, slow).Frame.Rows) != nativerunner.HolderRows; i++ {
		if i > 100 {
			t.Fatal("no full frame once the slow attach read again")
		}
	}
}

func TestHolderAttach_NoOutputMeansNoFramesNorTimers(t *testing.T) {
	c, _, stop, _ := attachTestHolder(t)
	a := attach(t, c)
	nextAttach(t, a)
	time.Sleep(5 * nativerunner.FrameInterval)
	if n := nativerunner.ArmedFrameTimers(); n != 0 {
		t.Errorf("%d pacing timers armed while idle; want none", n)
	}
	// The only message after the full frame is the exit.
	touch(t, stop)
	if m := nextAttach(t, a); m.Type != nativerunner.StreamExit {
		t.Errorf("after an idle child = %+v; want the exit", m)
	}
}

func TestHolderAttach_OversizedMessageClosesWithError(t *testing.T) {
	c, _, _, _ := attachTestHolder(t)
	a := attach(t, c)
	nextAttach(t, a)
	if err := nativerunner.SendOversizedHeader(a); err != nil {
		t.Fatal(err)
	}
	m, err := a.Next()
	if err == nil || !strings.Contains(err.Error(), "too big") {
		t.Fatalf("after an oversized message = %+v, %v; want a too-big error", m, err)
	}
	if m, err := a.Next(); err == nil {
		t.Errorf("after the error got %+v; want the attach closed", m)
	}
}

func TestHolderAttach_OlderHolderIsUnsupported(t *testing.T) {
	nativerunner.SetHolderOptionalOps(t, nil)
	c, _, _, _ := attachTestHolder(t)
	if _, err := c.Attach(); !errors.Is(err, nativerunner.ErrAttachUnsupported) {
		t.Errorf("Attach() on an older holder = %v; want ErrAttachUnsupported", err)
	}
	rows, _, err := c.Screen(0)
	if err != nil || !slices.Contains(rows, "first") {
		t.Errorf("screen op = %q, %v; want the snapshot to still answer", rows, err)
	}
}
