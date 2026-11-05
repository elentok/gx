package agents

import (
	"cmp"
	"errors"
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// AttachStream is the slice of nativerunner.HolderAttach the terminal panel
// uses.
type AttachStream interface {
	Next() (holder.AttachMessage, error)
	SendKey(k screen.Key) error
	Paste(text string) error
	Resize(cols, rows int) error
	Scroll(offset int) error
	Wheel(x, y int, down bool) error
	Mouse(x, y int, kind screen.MouseKind, button int) error
	Copy(start, end holder.LinePos) error
	Close() error
}

// attachDialer opens the attach of the holder serving a session dir, or, for
// a holder without attach, returns its screen rows instead.
type attachDialer func(dir string) (stream AttachStream, snapshot []string, err error)

// dialHolderAttach talks to holder.sock directly, not through the server
// (ADR 0037).
func dialHolderAttach(dir string) (AttachStream, []string, error) {
	c, err := holder.DialHolder(dir)
	if err != nil {
		return nil, nil, err
	}
	a, err := c.Attach()
	if errors.Is(err, holder.ErrAttachUnsupported) {
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
	// w sends every write to stream; Update only enqueues.
	w      *attachWriter
	closed bool
	// size is the last size sent to the holder.
	size [2]int
	// status is the session's status when the dial started; an attach that
	// ended with an error redials once the session's status moves on.
	status  wire.SessionStatus
	attempt int
	// retrying: a failed dial waits for its next attempt.
	retrying bool
	// ended: the stream is over (exit or error). An exit is never redialed
	// while the same session stays on screen.
	ended  bool
	exited bool
	err    error
	rows   []string
	cursor holder.FrameCursor
	// offset, scrollMax, scrollBase and newLines are the latest frame's
	// scroll state.
	offset, scrollMax, scrollBase, newLines int
	// mouse: the latest frame says the child reports the mouse.
	mouse bool
	// held is the button a forwarded press is waiting to be released from,
	// plus one; zero when none is held.
	held int
	// readOnly: keys and paste are dropped (a Queue or Design agent) until the
	// person confirms an unlock. A new attachment, so every new open, starts
	// read-only again.
	readOnly bool
}

func (a *attachment) close() {
	if a == nil || a.closed {
		return
	}
	a.closed = true
	if a.w != nil {
		a.w.close()
	}
	if a.stream != nil {
		_ = a.stream.Close()
	}
}

// live: the attach has a stream that hasn't ended.
func (a *attachment) live() bool {
	return a != nil && a.stream != nil && !a.ended
}

func (a *attachment) write(f func(AttachStream) error) {
	if a.live() {
		a.w.enqueue(f)
	}
}

type attachedMsg struct {
	a        *attachment
	stream   AttachStream
	snapshot []string
	err      error
}

type attachRetryMsg struct{ a *attachment }

// A Starting session's holder socket often doesn't exist yet when the tab
// dials, so the dial is retried with backoff, up to maxAttachRetries times.
const (
	maxAttachRetries = 6
	attachRetryBase  = 100 * time.Millisecond
)

type attachStreamMsg struct {
	a   *attachment
	msg holder.AttachMessage
	err error
}

// attachTarget is the session the panel shows live, or "" for none.
func (m Model) attachTarget() string {
	if m.client == nil || m.link == tabapi.ServerLinkDown || !m.loaded || m.term.sessionsDir == "" || m.cursor >= len(m.sessions) {
		return ""
	}
	if s := m.sessions[m.cursor]; s.Status.State != wire.SessionExited {
		return s.ID
	}
	return ""
}

// syncAttach keeps exactly the on-screen session attached: it closes the
// attach of a session no longer shown and dials the one now shown.
func (m Model) syncAttach() (Model, tea.Cmd) {
	want := m.attachTarget()
	if a := m.attach; a != nil && !a.closed && a.id == want && !a.redialDue(m.sessions[m.cursor]) {
		return m, nil
	}
	m.attach.close()
	m.attach = nil
	if want == "" {
		return m, nil
	}
	return m.dialAttach(0)
}

// redialDue: a, the selected session's attach, failed and the session's
// status has changed since it dialed.
func (a *attachment) redialDue(s wire.SessionInfo) bool {
	return a.ended && !a.exited && a.err != nil && a.status != s.Status
}

// dialAttach replaces the attach with a new dial of the selected session.
func (m Model) dialAttach(attempt int) (Model, tea.Cmd) {
	m.attach.close()
	s := m.sessions[m.cursor]
	a := &attachment{id: s.ID, status: s.Status, attempt: attempt, readOnly: s.Queue || s.Design}
	m.attach = a
	// A Queue agent's holder lives under the agents root, not the sessions dir.
	dir, dial := cmp.Or(s.Holder, filepath.Join(m.term.sessionsDir, s.ID)), m.dial
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

// current: msg is about a, the attach on screen, and a is still open.
func (a *attachment) current(msgA *attachment) bool {
	return a != nil && a == msgA && !a.closed
}

// updateAttached takes the dial's outcome; a is the attach on screen.
func (a *attachment) updateAttached(msg attachedMsg) tea.Cmd {
	if !a.current(msg.a) {
		// The person moved on while the dial was in flight.
		if msg.stream != nil {
			_ = msg.stream.Close()
		}
		return nil
	}
	if msg.err != nil || msg.stream == nil {
		a.ended, a.err, a.rows = true, msg.err, msg.snapshot
		if msg.err != nil && a.status.State == wire.SessionStarting && a.attempt < maxAttachRetries {
			a.retrying = true
			return tea.Tick(attachRetryBase<<a.attempt, func(time.Time) tea.Msg { return attachRetryMsg{a: a} })
		}
		return nil
	}
	a.stream = msg.stream
	a.w = startAttachWriter(msg.stream)
	return readAttach(a)
}

// streamResult is what a stream message asks of the tab beyond the attach.
type streamResult struct {
	// Exited: the session ended, so the tab drops focus and copy mode.
	Exited bool
	// Copied is the text the session put on the clipboard, nil for none.
	Copied *string
}

// updateStream applies one stream message; a is the attach on screen.
func (a *attachment) updateStream(msg attachStreamMsg) (streamResult, tea.Cmd) {
	if !a.current(msg.a) {
		return streamResult{}, nil
	}
	if msg.err != nil {
		a.ended, a.err = true, msg.err
		// A failed write closes the stream; its error, not the read's, is why.
		if err := a.w.failure(); err != nil {
			a.err = err
		}
		return streamResult{}, nil
	}
	switch msg.msg.Type {
	case holder.StreamFrame:
		f := msg.msg.Frame
		a.rows = applyFrame(a.rows, f)
		a.cursor = f.Cursor
		a.offset, a.scrollMax, a.scrollBase, a.newLines = f.ScrollOffset, f.ScrollMax, f.ScrollBase, f.NewLines
		a.mouse = f.Mouse
	case holder.StreamExit:
		a.ended, a.exited = true, true
		return streamResult{Exited: true}, nil
	case holder.StreamCopy:
		return streamResult{Copied: &msg.msg.Text}, readAttach(a)
	}
	return streamResult{}, readAttach(a)
}

// applyFrame returns rows with the frame's changed rows written over them, or
// just the frame's rows for a full frame.
func applyFrame(rows []string, f holder.Frame) []string {
	if f.Full {
		rows = nil
	}
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

// sendKey types k into the attached session.
func (a *attachment) sendKey(k tea.KeyPressMsg) {
	if a.live() && !a.readOnly {
		a.snapToBottom()
		key := screen.Key{Code: k.Code, Text: k.Text, Mod: k.Mod}
		a.write(func(s AttachStream) error { return s.SendKey(key) })
	}
}

func (a *attachment) sendPaste(text string) tea.Cmd {
	if len(text) > maxPasteBytes {
		return notify.Warning("paste too large (over 1 MB), not sent")
	}
	if a.live() && !a.readOnly {
		a.snapToBottom()
		a.write(func(s AttachStream) error { return s.Paste(text) })
	}
	return nil
}

func (a *attachment) scroll(offset int) {
	a.offset = offset
	a.write(func(s AttachStream) error { return s.Scroll(offset) })
}

// snapToBottom returns a scrolled-back view to the live screen, so input
// typed while reading the scrollback is never sent blind.
func (a *attachment) snapToBottom() {
	if a.offset > 0 {
		a.newLines = 0
		a.scroll(0)
	}
}

// wheel handles the wheel over the panel, whichever pane has focus: a child
// that reports the mouse gets it over its screen cell (x, y; inCell false off
// the screen), as with tmux, since one in the alternate screen (claude) has no
// scrollback; otherwise it scrolls the panel's view through the holder.
func (a *attachment) wheel(dir, x, y int, inCell bool) {
	if !a.live() {
		return
	}
	if a.mouse && a.offset == 0 {
		if inCell {
			a.write(func(s AttachStream) error { return s.Wheel(x, y, dir > 0) })
		}
		return
	}
	// Set locally too, so wheel ticks faster than frames still add up.
	if offset := max(0, min(a.offset-dir*ui.WheelScrollLines, a.scrollMax)); offset != a.offset {
		a.scroll(offset)
	}
}

// mouseButton maps a bubbletea button to the AttachMouse button, false for
// one the child is not sent.
func mouseButton(b tea.MouseButton) (int, bool) {
	switch b {
	case tea.MouseLeft:
		return 0, true
	case tea.MouseMiddle:
		return 1, true
	case tea.MouseRight:
		return 2, true
	}
	return 0, false
}

// forwardPress sends a click over the child's screen to a child that reports
// the mouse, as the wheel is (see wheel). It reports whether the child took
// it; a click on the border or the info rows stays with gx. cell maps the
// mouse to the screen cell under it.
func (a *attachment) forwardPress(mouse tea.Mouse, cell func(tea.Mouse) (x, y int, ok bool)) bool {
	button, ok := mouseButton(mouse.Button)
	if !ok || !a.live() || a.readOnly || !a.mouse || a.offset != 0 {
		return false
	}
	x, y, ok := cell(mouse)
	if !ok {
		return false
	}
	a.held = button + 1
	a.write(func(s AttachStream) error { return s.Mouse(x, y, screen.MousePress, button) })
	return true
}

// forwardMove follows a held press with the pointer, clamped to the screen so
// the child sees the drag end where it left the panel. clamp maps the mouse
// to the nearest screen cell.
func (a *attachment) forwardMove(mouse tea.Mouse, kind screen.MouseKind, clamp func(tea.Mouse) (x, y int)) {
	if !a.live() || a.held == 0 {
		return
	}
	button := a.held - 1
	x, y := clamp(mouse)
	if kind == screen.MouseRelease {
		a.held = 0
	}
	a.write(func(s AttachStream) error { return s.Mouse(x, y, kind, button) })
}

// resize gives the session the panel's size. It runs on every focus, not only
// on change: the pty follows the viewer that last focused or typed, and
// another viewer may have resized it since. Typing needs no Resize, as the
// holder claims this viewer's last size on input.
func (a *attachment) resize(cols, rows int) {
	if a.live() {
		a.size = [2]int{cols, rows}
		a.write(func(s AttachStream) error { return s.Resize(cols, rows) })
	}
}

// syncSize sends the panel's size when it changed.
func (a *attachment) syncSize(cols, rows int) {
	if a.live() && a.size != [2]int{cols, rows} {
		a.resize(cols, rows)
	}
}
