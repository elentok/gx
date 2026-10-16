// Package screen is gx's terminal emulator: it turns an agent's raw PTY
// output into screen and scrollback rows, plain or styled, and encodes input
// for the modes the agent turned on. It is the only package that imports
// charmbracelet/x/vt, so swapping the emulator (or pointing a `replace` at a
// fork) stays local to this file.
package screen

import (
	"io"
	"strings"
	"sync"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

// scrollbackLines is fixed, not a setting (ADR 0037).
const scrollbackLines = 10_000

// Key is a key press to encode for the child.
type Key = uv.Key

// KeyUp and friends name the special keys a Key can carry.
const (
	KeyUp    = uv.KeyUp
	KeyDown  = uv.KeyDown
	KeyLeft  = uv.KeyLeft
	KeyRight = uv.KeyRight
)

// Cursor is the cursor's position on the visible screen.
type Cursor struct {
	X, Y    int
	Visible bool
}

// Screen is a concurrency-safe virtual terminal fed with a child's raw
// output.
type Screen struct {
	mu      sync.Mutex
	emu     *vt.Emulator
	drained chan struct{}
	// cursorVisible and allDirty are set by emulator callbacks, which run
	// inside calls that already hold mu.
	cursorVisible bool
	// allDirty covers what the touched lines miss: a resize reallocates
	// them, and a screen switch shows a buffer whose marks are stale.
	allDirty bool
}

// NewScreen returns a cols×rows screen whose replies to terminal queries are
// discarded. Call Close when done.
func NewScreen(cols, rows int) *Screen {
	return NewScreenWithReplies(cols, rows, nil)
}

// NewScreenWithReplies returns a cols×rows screen that writes its replies to
// terminal queries, and the input from SendKey and Paste, to replies. A nil
// replies discards them. Call Close when done.
func NewScreenWithReplies(cols, rows int, replies io.Writer) *Screen {
	if replies == nil {
		replies = io.Discard
	}
	emu := vt.NewEmulator(cols, rows)
	emu.SetScrollbackSize(scrollbackLines)
	s := &Screen{emu: emu, drained: make(chan struct{}), cursorVisible: true}
	emu.SetCallbacks(vt.Callbacks{
		CursorVisibility: func(visible bool) { s.cursorVisible = visible },
		AltScreen:        func(bool) { s.allDirty = true },
	})
	// The emulator writes replies and encoded input into a pipe, and an
	// undrained pipe blocks the next Write that triggers one.
	go func() {
		defer close(s.drained)
		_, _ = io.Copy(replies, emu)
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
	s.allDirty = true
}

// Rows returns the visible screen as text, one entry per row, with trailing
// blank rows dropped.
func (s *Screen) Rows() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Split(s.emu.String(), "\n")
}

// StyledRows returns every visible row with its styles as ANSI sequences.
func (s *Screen) StyledRows() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Split(s.emu.Render(), "\n")
}

// Cursor returns the cursor's position and visibility.
func (s *Screen) Cursor() Cursor {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.emu.CursorPosition()
	return Cursor{X: p.X, Y: p.Y, Visible: s.cursorVisible}
}

// DirtyRows returns the indexes of the visible rows changed since the last
// ClearDirty, in order.
func (s *Screen) DirtyRows() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var rows []int
	if s.allDirty {
		for y := range s.emu.Height() {
			rows = append(rows, y)
		}
		return rows
	}
	for y, line := range s.emu.Touched() {
		if line != nil && y < s.emu.Height() {
			rows = append(rows, y)
		}
	}
	return rows
}

// ClearDirty marks every visible row clean.
func (s *Screen) ClearDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allDirty = false
	// The slice shares its backing array with the emulator's buffer.
	clear(s.emu.Touched())
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

// ScrollbackLen returns how many lines the main screen's scrollback holds.
func (s *Screen) ScrollbackLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emu.ScrollbackLen()
}

// StyledScrollback returns scrollback line i (0 is the oldest) with its
// styles, or "" when i is out of range.
func (s *Screen) StyledScrollback(i int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emu.Scrollback().Line(i).Render()
}

// SendKey encodes k for the modes the child turned on and writes it to the
// replies writer.
func (s *Screen) SendKey(k Key) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emu.SendKey(uv.KeyPressEvent(k))
}

// Paste writes text to the replies writer, bracketed when the child turned on
// bracketed paste.
func (s *Screen) Paste(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emu.Paste(text)
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
