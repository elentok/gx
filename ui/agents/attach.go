package agents

import (
	"errors"
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

// AttachStream is the slice of nativerunner.HolderAttach the terminal panel
// uses.
type AttachStream interface {
	Next() (nativerunner.AttachMessage, error)
	SendKey(k screen.Key) error
	Paste(text string) error
	Resize(cols, rows int) error
	Scroll(offset int) error
	Copy(start, end nativerunner.LinePos) error
	Close() error
}

// attachDialer opens the attach of the holder serving a session dir, or, for
// a holder without attach, returns its screen rows instead.
type attachDialer func(dir string) (stream AttachStream, snapshot []string, err error)

// dialHolderAttach talks to holder.sock directly, not through the server
// (ADR 0037).
func dialHolderAttach(dir string) (AttachStream, []string, error) {
	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		return nil, nil, err
	}
	a, err := c.Attach()
	if errors.Is(err, nativerunner.ErrAttachUnsupported) {
		rows, _, err := c.Screen(0)
		return nil, rows, err
	}
	if err != nil {
		return nil, nil, err
	}
	return a, nil, nil
}

// attachment is the on-screen session's attach. It is shared by every copy of
// the Model, so leaving the tab can close it without an Update.
type attachment struct {
	id string
	// stream is nil until the dial returns, and for a snapshot.
	stream AttachStream
	closed bool
	// ended: the stream is over (exit or error), so it must not be redialed
	// while the same session stays on screen.
	ended  bool
	exited bool
	err    error
	rows   []string
	// offset, scrollMax and newLines are the latest frame's scroll state.
	offset, scrollMax, newLines int
}

func (a *attachment) close() {
	if a == nil || a.closed {
		return
	}
	a.closed = true
	if a.stream != nil {
		_ = a.stream.Close()
	}
}

type attachedMsg struct {
	a        *attachment
	stream   AttachStream
	snapshot []string
	err      error
}

type attachStreamMsg struct {
	a   *attachment
	msg nativerunner.AttachMessage
	err error
}

// attachTarget is the session the panel shows live, or "" for none.
func (m Model) attachTarget() string {
	if m.client == nil || m.link == ticketsui.ServerLinkDown || !m.loaded || m.sessionsDir == "" || m.cursor >= len(m.sessions) {
		return ""
	}
	if s := m.sessions[m.cursor]; s.Status.State != server.SessionExited {
		return s.ID
	}
	return ""
}

// syncAttach keeps exactly the on-screen session attached: it closes the
// attach of a session no longer shown and dials the one now shown.
func (m Model) syncAttach() (Model, tea.Cmd) {
	want := m.attachTarget()
	if m.attach != nil && !m.attach.closed && m.attach.id == want {
		return m, nil
	}
	m.attach.close()
	m.attach = nil
	if want == "" {
		return m, nil
	}
	a := &attachment{id: want}
	m.attach = a
	dir, dial := filepath.Join(m.sessionsDir, want), m.dial
	return m, func() tea.Msg {
		stream, snapshot, err := dial(dir)
		return attachedMsg{a: a, stream: stream, snapshot: snapshot, err: err}
	}
}

func readAttach(a *attachment) tea.Cmd {
	stream := a.stream
	return func() tea.Msg {
		msg, err := stream.Next()
		return attachStreamMsg{a: a, msg: msg, err: err}
	}
}

func (m Model) updateAttached(msg attachedMsg) (Model, tea.Cmd) {
	a := msg.a
	if a != m.attach || a.closed {
		// The person moved on while the dial was in flight.
		if msg.stream != nil {
			_ = msg.stream.Close()
		}
		return m, nil
	}
	if msg.err != nil || msg.stream == nil {
		a.ended, a.err, a.rows = true, msg.err, msg.snapshot
		return m, nil
	}
	a.stream = msg.stream
	if m.terminalFocused {
		m.resizeAttach()
	}
	return m, readAttach(a)
}

func (m Model) updateAttachStream(msg attachStreamMsg) (Model, tea.Cmd) {
	a := msg.a
	if a != m.attach || a.closed {
		return m, nil
	}
	if msg.err != nil {
		a.ended, a.err = true, msg.err
		return m, nil
	}
	switch msg.msg.Type {
	case nativerunner.StreamFrame:
		f := msg.msg.Frame
		a.rows = applyFrame(a.rows, f)
		a.offset, a.scrollMax, a.newLines = f.ScrollOffset, f.ScrollMax, f.NewLines
	case nativerunner.StreamExit:
		a.ended, a.exited = true, true
		m.terminalFocused = false
		// Shown until the server reports the exit and the saved last screen loads.
		m.lastScreen = lastScreen{id: a.id, rows: a.rows}
		m.copy = nil
		return m, nil
	case nativerunner.StreamCopy:
		return m, tea.Batch(m.writeCopied(msg.msg.Text), readAttach(a))
	}
	return m, readAttach(a)
}

// applyFrame returns rows with the frame's changed rows written over them.
func applyFrame(rows []string, f nativerunner.Frame) []string {
	rows = slices.Clone(rows)
	for _, r := range f.Rows {
		if r.Index >= len(rows) {
			rows = append(rows, make([]string, r.Index+1-len(rows))...)
		}
		rows[r.Index] = r.ANSI
	}
	return rows
}

// maxPasteBytes caps a paste so a stray huge clipboard can't flood the session.
const maxPasteBytes = 1 << 20

// sendKey types k into the attached session. It writes synchronously so keys
// reach the session in the order they were pressed.
func (m Model) sendKey(k tea.KeyPressMsg) {
	if a := m.attach; a != nil && a.stream != nil && !a.ended {
		m.resizeAttach()
		a.snapToBottom()
		_ = a.stream.SendKey(screen.Key{Code: k.Code, Text: k.Text, Mod: k.Mod})
	}
}

func (m Model) sendPaste(text string) tea.Cmd {
	if len(text) > maxPasteBytes {
		return notify.Warning("paste too large (over 1 MB), not sent")
	}
	if a := m.attach; a != nil && a.stream != nil && !a.ended {
		m.resizeAttach()
		a.snapToBottom()
		_ = a.stream.Paste(text)
	}
	return nil
}

// snapToBottom returns a scrolled-back view to the live screen, so input
// typed while reading the scrollback is never sent blind.
func (a *attachment) snapToBottom() {
	if a.offset > 0 {
		a.offset, a.newLines = 0, 0
		_ = a.stream.Scroll(0)
	}
}

// updateWheel scrolls the panel's view through the holder when the wheel is
// over the panel, whichever pane has focus.
func (m Model) updateWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	dir, ok := ui.WheelDirection(msg)
	a := m.attach
	if !ok || !m.overPanel(msg.Mouse()) || a == nil || a.stream == nil || a.ended {
		return m, nil
	}
	// Set locally too, so wheel ticks faster than frames still add up.
	if offset := max(0, min(a.offset-dir*ui.WheelScrollLines, a.scrollMax)); offset != a.offset {
		a.offset = offset
		_ = a.stream.Scroll(offset)
	}
	return m, nil
}

func (m Model) overPanel(mouse tea.Mouse) bool {
	left := m.width - m.mainWidth()
	return mouse.X >= left && mouse.X < m.width && mouse.Y >= 0 && mouse.Y < m.height
}

// resizeAttach gives the session the panel's size. It runs on focus and
// before every input, not only on change: the pty follows the viewer that
// last focused or typed, and another viewer may have resized it since.
func (m Model) resizeAttach() {
	if a := m.attach; a != nil && a.stream != nil && !a.ended {
		cols, rows := m.screenSize()
		_ = a.stream.Resize(cols, rows)
	}
}
