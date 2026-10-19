// Package screen is gx's terminal emulator: it turns an agent's raw PTY
// output into screen and scrollback rows, plain or styled, and encodes input
// for the modes the agent turned on. It is the only package that imports
// charmbracelet/x/vt, so swapping the emulator (or pointing a `replace` at a
// fork) stays local to this file.
package screen

import (
	"fmt"
	"io"
	"strings"
	"sync"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

// scrollbackLines is fixed, not a setting (ADR 0037).
const scrollbackLines = 10_000

// The emulator drops its oldest line silently once full, so it holds up to
// scrollbackSlack lines past scrollbackLines, which Write trims after each
// writeChunk: its growth is then the count of lines pushed.
const (
	scrollbackSlack = scrollbackLines
	writeChunk      = 4096
)

// Key is a key press to encode for the child.
type Key = uv.Key

// KeyMod is a Key's modifier bits.
type KeyMod = uv.KeyMod

// KeyUp and friends name the special keys a Key can carry.
const (
	KeyUp    = uv.KeyUp
	KeyDown  = uv.KeyDown
	KeyLeft  = uv.KeyLeft
	KeyRight = uv.KeyRight
	KeyEnter = uv.KeyEnter
)

// ModShift and friends name a Key's modifier bits.
const (
	ModShift    = uv.ModShift
	ModAlt      = uv.ModAlt
	ModCtrl     = uv.ModCtrl
	ModCapsLock = uv.ModCapsLock
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
	// pushed counts the lines ever pushed to the main screen's scrollback.
	pushed int
	// trimmed counts the lines trimmed off the scrollback's top, so a line
	// number counts from the oldest line ever pushed and keeps naming the same
	// line while the scrollback trims.
	trimmed int
	// mouse is the mouse reporting modes the child turned on.
	mouse map[ansi.Mode]bool
	st    stFilter
	buf   []byte
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
	emu.SetScrollbackSize(scrollbackLines + scrollbackSlack)
	s := &Screen{emu: emu, drained: make(chan struct{}), cursorVisible: true, mouse: map[ansi.Mode]bool{}}
	emu.SetCallbacks(vt.Callbacks{
		CursorVisibility: func(visible bool) { s.cursorVisible = visible },
		AltScreen:        func(bool) { s.allDirty = true },
		EnableMode:       func(m ansi.Mode) { s.setMouseMode(m, true) },
		DisableMode:      func(m ansi.Mode) { s.setMouseMode(m, false) },
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
	n := 0
	for len(p) > 0 {
		chunk := p[:min(len(p), writeChunk)]
		s.buf = append(s.buf[:0], chunk...)
		s.st.filter(s.buf)
		before := s.emu.ScrollbackLen()
		m, err := s.emu.Write(s.buf)
		n += m
		s.pushed += max(0, s.emu.ScrollbackLen()-before)
		s.trimScrollback()
		if err != nil {
			return n, err
		}
		p = p[len(chunk):]
	}
	return n, nil
}

func (s *Screen) trimScrollback() {
	if sb := s.emu.Scrollback(); sb.Len() > scrollbackLines {
		s.trimmed += sb.Len() - scrollbackLines
		sb.SetMaxLines(scrollbackLines)
		sb.SetMaxLines(scrollbackLines + scrollbackSlack)
	}
}

// Resize changes the screen size.
func (s *Screen) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emu.Resize(cols, rows)
	s.trimScrollback()
	s.allDirty = true
}

// Pushed returns how many lines have ever scrolled off the main screen into
// the scrollback, counting those since dropped.
func (s *Screen) Pushed() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pushed
}

// ScrollbackSpan returns how many lines were trimmed off the scrollback's top
// and how many it holds, read together.
func (s *Screen) ScrollbackSpan() (trimmed, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trimmed, s.emu.ScrollbackLen()
}

// StyledView returns n styled rows of the scrollback followed by the screen,
// starting offset lines above the screen's top, with "" past the end.
// offset is clamped to scrollMax, the scrollback's length; trimmed is as in
// ScrollbackSpan.
func (s *Screen) StyledView(offset, n int) (rows []string, trimmed, scrollMax int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sb := s.emu.Scrollback()
	trimmed, scrollMax = s.trimmed, sb.Len()
	scr := strings.Split(s.emu.Render(), "\n")
	top := scrollMax - min(offset, scrollMax)
	rows = make([]string, n)
	for i := range rows {
		switch l := top + i; {
		case l < scrollMax:
			rows[i] = sb.Line(l).Render()
		case l-scrollMax < len(scr):
			rows[i] = scr[l-scrollMax]
		}
	}
	return rows, trimmed, scrollMax
}

// Text returns the plain text from column startCol of line startLine up to,
// not including, column endCol of line endLine, each line's trailing spaces
// trimmed. Lines count from the oldest line ever pushed to the scrollback,
// trimmed ones included, the screen's rows following the scrollback's. A
// trimmed position is clamped to the oldest retained line's start.
func (s *Screen) Text(startLine, startCol, endLine, endCol int) string {
	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		startLine, startCol, endLine, endCol = endLine, endCol, startLine, startCol
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	startLine -= s.trimmed
	endLine -= s.trimmed
	if startLine < 0 {
		startLine, startCol = 0, 0
	}
	if endLine < 0 {
		endLine, endCol = 0, 0
	}
	sb := s.emu.Scrollback()
	n := sb.Len()
	scr := strings.Split(s.emu.String(), "\n")
	var lines []string
	for l := max(startLine, 0); l <= min(endLine, n+len(scr)-1); l++ {
		var text string
		if l < n {
			text = sb.Line(l).String()
		} else {
			text = scr[l-n]
		}
		from, to := 0, ansi.StringWidth(text)
		if l == startLine {
			from = startCol
		}
		if l == endLine {
			to = endCol
		}
		lines = append(lines, strings.TrimRight(ansi.Cut(text, from, to), " "))
	}
	return strings.Join(lines, "\n")
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
	return s.dirtyRows()
}

func (s *Screen) dirtyRows() []int {
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

// TakeDirty returns DirtyRows and marks them clean in one step, so no change
// slips in between the two.
func (s *Screen) TakeDirty() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := s.dirtyRows()
	s.clearDirty()
	return rows
}

// ClearDirty marks every visible row clean.
func (s *Screen) ClearDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearDirty()
}

func (s *Screen) clearDirty() {
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
	k.Mod &^= uv.ModCapsLock | uv.ModNumLock | uv.ModScrollLock
	// The emulator drops a key with any modifier but Alt, so text typed with
	// Shift (h with Shift is "H") goes as its runes.
	if k.Text != "" && k.Mod&^(uv.ModShift|uv.ModAlt) == 0 {
		for _, r := range k.Text {
			s.emu.SendKey(uv.KeyPressEvent{Code: r, Mod: k.Mod & uv.ModAlt})
		}
		return
	}
	// The emulator has no encoding for Shift+Enter, so it goes as Alt+Enter
	// (ESC CR), which agents such as Claude Code read as a line break.
	if k.Code == uv.KeyEnter && k.Mod&^uv.ModAlt == uv.ModShift {
		k.Mod = uv.ModAlt
	}
	if seq, ok := modifiedNavSeq(k); ok {
		s.emu.SendText(seq)
		return
	}
	s.emu.SendKey(uv.KeyPressEvent(k))
}

// navFinals is the CSI final byte of the cursor keys, Home and End, and
// navTildes the CSI number of the keys that end in "~".
var (
	navFinals = map[rune]byte{uv.KeyUp: 'A', uv.KeyDown: 'B', uv.KeyRight: 'C', uv.KeyLeft: 'D', uv.KeyHome: 'H', uv.KeyEnd: 'F'}
	navTildes = map[rune]int{uv.KeyInsert: 2, uv.KeyDelete: 3, uv.KeyPgUp: 5, uv.KeyPgDown: 6}
)

// modifiedNavSeq encodes a navigation key with Shift, Alt or Ctrl the way
// xterm does ("\x1b[1;3D" is Alt+Left). The emulator drops such keys with
// Shift or Ctrl, and sends Alt as a leading ESC, which agents such as Claude
// Code do not read as a word jump.
func modifiedNavSeq(k Key) (string, bool) {
	mods := k.Mod & (uv.ModShift | uv.ModAlt | uv.ModCtrl)
	if mods == 0 {
		return "", false
	}
	param := 1
	if mods&uv.ModShift != 0 {
		param++
	}
	if mods&uv.ModAlt != 0 {
		param += 2
	}
	if mods&uv.ModCtrl != 0 {
		param += 4
	}
	if final, ok := navFinals[k.Code]; ok {
		return fmt.Sprintf("\x1b[1;%d%c", param, final), true
	}
	if n, ok := navTildes[k.Code]; ok {
		return fmt.Sprintf("\x1b[%d;%d~", n, param), true
	}
	return "", false
}

func (s *Screen) setMouseMode(m ansi.Mode, on bool) {
	switch m {
	case ansi.ModeMouseX10, ansi.ModeMouseNormal, ansi.ModeMouseHighlight, ansi.ModeMouseButtonEvent, ansi.ModeMouseAnyEvent:
		if on {
			s.mouse[m] = true
		} else {
			delete(s.mouse, m)
		}
	}
}

// MouseReporting reports whether the child turned on mouse reporting, so it
// takes the wheel as mouse events.
func (s *Screen) MouseReporting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.mouse) > 0
}

// SendWheel encodes a wheel turn over cell (x, y), down or up, for the
// child's mouse modes and writes it to the replies writer. Without mouse
// reporting it writes nothing.
func (s *Screen) SendWheel(x, y int, down bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	button := vt.MouseWheelUp
	if down {
		button = vt.MouseWheelDown
	}
	s.emu.SendMouse(vt.MouseWheel{X: x, Y: y, Button: button})
}

// MouseKind is the phase of a button event SendMouse encodes.
type MouseKind int

const (
	MousePress MouseKind = iota
	MouseRelease
	MouseDrag
)

// SendMouse encodes a left, middle or right button event over cell (x, y) for
// the child's mouse modes and writes it to the replies writer. Without mouse
// reporting it writes nothing.
func (s *Screen) SendMouse(x, y int, kind MouseKind, button int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := vt.MouseLeft
	switch button {
	case 1:
		b = vt.MouseMiddle
	case 2:
		b = vt.MouseRight
	}
	switch kind {
	case MouseRelease:
		s.emu.SendMouse(vt.MouseRelease{X: x, Y: y, Button: b})
	case MouseDrag:
		s.emu.SendMouse(vt.MouseMotion{X: x, Y: y, Button: b})
	default:
		s.emu.SendMouse(vt.MouseClick{X: x, Y: y, Button: b})
	}
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
