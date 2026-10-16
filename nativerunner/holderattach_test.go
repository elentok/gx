package nativerunner_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
		if m.Type != nativerunner.StreamFrame || len(m.Frame.Rows) != nativerunner.HolderRows {
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
		if m.Type != nativerunner.StreamFrame || !slices.Equal(rowIndexes(m.Frame), []int{1}) {
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

func TestHolderAttach_NoOutputMeansNoFrames(t *testing.T) {
	c, _, stop, _ := attachTestHolder(t)
	a := attach(t, c)
	nextAttach(t, a)
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
