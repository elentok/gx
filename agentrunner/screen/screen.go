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
	mu  sync.Mutex
	emu *vt.Emulator
}

// NewScreen returns a cols×rows screen. Call Close when done.
func NewScreen(cols, rows int) *Screen {
	emu := vt.NewEmulator(cols, rows)
	// The emulator writes its replies to terminal queries into a pipe; an
	// undrained pipe blocks the next Write that triggers one.
	go func() { _, _ = io.Copy(io.Discard, emu) }()
	return &Screen{emu: emu}
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

// Close stops the emulator.
func (s *Screen) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emu.Close()
}
