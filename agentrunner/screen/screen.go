// Package screen is gx's terminal emulator: it turns an agent's raw PTY
// output into plain-text screen and scrollback rows. It is the only package
// that imports charmbracelet/x/vt, so swapping the emulator (or pointing a
// `replace` at a fork) stays local to this file.
package screen

import (
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/x/vt"
)

// Screen is a concurrency-safe virtual terminal fed with a child's raw
// output.
type Screen struct {
	mu      sync.Mutex
	emu     *vt.Emulator
	drained chan struct{}
}

// NewScreen returns a cols×rows screen. Call Close when done.
func NewScreen(cols, rows int) *Screen {
	emu := vt.NewEmulator(cols, rows)
	s := &Screen{emu: emu, drained: make(chan struct{})}
	// The emulator writes its replies to terminal queries (cursor position,
	// device attributes) into a pipe, and an undrained pipe blocks the next
	// Write that triggers one. The replies are discarded on purpose: the real
	// terminal attached to the PTY answers the child's queries, not this
	// screen.
	go func() {
		defer close(s.drained)
		_, _ = io.Copy(io.Discard, emu)
	}()
	return s
}

// Write feeds raw output bytes to the emulator.
func (s *Screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emu.Write(p)
}

// Resize changes the screen size.
func (s *Screen) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emu.Resize(cols, rows)
}

// Rows returns the visible screen as text, one entry per row, with trailing
// blank rows dropped.
func (s *Screen) Rows() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Split(s.emu.String(), "\n")
}

// ScrollbackTail returns up to the last n rows that scrolled off the main
// screen, oldest first. The alternate screen has no scrollback.
func (s *Screen) ScrollbackTail(n int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := s.emu.Scrollback().Lines()
	lines = lines[max(0, len(lines)-n):]
	rows := make([]string, len(lines))
	for i, l := range lines {
		rows[i] = strings.TrimRight(l.String(), " ")
	}
	return rows
}

// Close stops the drain goroutine, waits for it to exit, then stops the
// emulator.
func (s *Screen) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// emu.Read checks the emulator's closed flag without a lock, so
	// emu.Close must not run until the drain goroutine is gone. Closing the
	// reply pipe first ends its io.Copy. The unchecked assertion panics
	// rather than hangs if the emulator ever stops handing back a closer.
	_ = s.emu.InputPipe().(io.Closer).Close()
	<-s.drained
	return s.emu.Close()
}
