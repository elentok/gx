package terminal

import (
	"errors"
	"slices"
	"time"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
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

// AttachDialer opens the attach of the holder serving a session dir, or, for
// a holder without attach, returns its screen rows instead.
type AttachDialer func(dir string) (stream AttachStream, snapshot []string, err error)

// DialHolderAttach talks to holder.sock directly, not through the server
// (ADR 0037).
func DialHolderAttach(dir string) (AttachStream, []string, error) {
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

// Attachment is the on-screen session's attach. It is shared by every copy of
// the Model, so leaving the tab can close it without an Update.
type Attachment struct {
	ID string
	// Stream is nil until the dial returns, and for a snapshot.
	Stream AttachStream
	// w sends every write to stream; Update only enqueues.
	w      *attachWriter
	Closed bool
	// size is the last size sent to the holder.
	size [2]int
	// Status is the session's Status when the dial started; an attach that
	// ended with an error redials once the session's Status moves on.
	Status  wire.SessionStatus
	Attempt int
	// retrying: a failed dial waits for its next attempt.
	retrying bool
	// Ended: the stream is over (exit or error). An exit is never redialed
	// while the same session stays on screen.
	Ended  bool
	exited bool
	err    error
	Rows   []string
	cursor holder.FrameCursor
	// offset, scrollMax, scrollBase and newLines are the latest frame's
	// scroll state.
	offset, scrollMax, scrollBase, newLines int
	// mouse: the latest frame says the child reports the mouse.
	mouse bool
	// held is the button a forwarded press is waiting to be released from,
	// plus one; zero when none is held.
	held int
	// ReadOnly: keys and paste are dropped (a Queue or Design agent) until the
	// person confirms an unlock. A new attachment, so every new open, starts
	// read-only again.
	ReadOnly bool
}

func (a *Attachment) Close() {
	if a == nil || a.Closed {
		return
	}
	a.Closed = true
	if a.w != nil {
		a.w.close()
	}
	if a.Stream != nil {
		_ = a.Stream.Close()
	}
}

// Live: the attach has a stream that hasn't ended.
func (a *Attachment) Live() bool {
	return a != nil && a.Stream != nil && !a.Ended
}

func (a *Attachment) write(f func(AttachStream) error) {
	if a.Live() {
		a.w.enqueue(f)
	}
}

type AttachedMsg struct {
	A        *Attachment
	Stream   AttachStream
	Snapshot []string
	Err      error
}

type AttachRetryMsg struct{ A *Attachment }

// A Starting session's holder socket often doesn't exist yet when the tab
// dials, so the dial is retried with backoff, up to MaxAttachRetries times.
const (
	MaxAttachRetries = 6
	attachRetryBase  = 100 * time.Millisecond
)

type AttachStreamMsg struct {
	A   *Attachment
	msg holder.AttachMessage
	err error
}

// RedialDue: a, the selected session's attach, failed and the session's
// status has changed since it dialed.
func (a *Attachment) RedialDue(s wire.SessionInfo) bool {
	return a.Ended && !a.exited && a.err != nil && a.Status != s.Status
}

// WaitWrites blocks until every queued write has been sent, for tests.
func (a *Attachment) WaitWrites() {
	if a != nil && a.w != nil {
		a.w.pending.Wait()
	}
}

func ReadAttach(a *Attachment) tea.Cmd {
	stream := a.Stream
	return func() tea.Msg {
		msg, err := stream.Next()
		return AttachStreamMsg{A: a, msg: msg, err: err}
	}
}

// Current: msg is about a, the attach on screen, and a is still open.
func (a *Attachment) Current(msgA *Attachment) bool {
	return a != nil && a == msgA && !a.Closed
}

// UpdateAttached takes the dial's outcome; a is the attach on screen.
func (a *Attachment) UpdateAttached(msg AttachedMsg) tea.Cmd {
	if !a.Current(msg.A) {
		// The person moved on while the dial was in flight.
		if msg.Stream != nil {
			_ = msg.Stream.Close()
		}
		return nil
	}
	if msg.Err != nil || msg.Stream == nil {
		a.Ended, a.err, a.Rows = true, msg.Err, msg.Snapshot
		if msg.Err != nil && a.Status.State == wire.SessionStarting && a.Attempt < MaxAttachRetries {
			a.retrying = true
			return tea.Tick(attachRetryBase<<a.Attempt, func(time.Time) tea.Msg { return AttachRetryMsg{A: a} })
		}
		return nil
	}
	a.Stream = msg.Stream
	a.w = startAttachWriter(msg.Stream)
	return ReadAttach(a)
}

// StreamResult is what a stream message asks of the tab beyond the attach.
type StreamResult struct {
	// Exited: the session ended, so the tab drops focus and copy mode.
	Exited bool
	// Copied is the text the session put on the clipboard, nil for none.
	Copied *string
}

// UpdateStream applies one stream message; a is the attach on screen.
func (a *Attachment) UpdateStream(msg AttachStreamMsg) (StreamResult, tea.Cmd) {
	if !a.Current(msg.A) {
		return StreamResult{}, nil
	}
	if msg.err != nil {
		a.Ended, a.err = true, msg.err
		// A failed write closes the stream; its error, not the read's, is why.
		if err := a.w.failure(); err != nil {
			a.err = err
		}
		return StreamResult{}, nil
	}
	switch msg.msg.Type {
	case holder.StreamFrame:
		f := msg.msg.Frame
		a.Rows = applyFrame(a.Rows, f)
		a.cursor = f.Cursor
		a.offset, a.scrollMax, a.scrollBase, a.newLines = f.ScrollOffset, f.ScrollMax, f.ScrollBase, f.NewLines
		a.mouse = f.Mouse
	case holder.StreamExit:
		a.Ended, a.exited = true, true
		return StreamResult{Exited: true}, nil
	case holder.StreamCopy:
		return StreamResult{Copied: &msg.msg.Text}, ReadAttach(a)
	}
	return StreamResult{}, ReadAttach(a)
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

// MaxPasteBytes caps a paste so a stray huge clipboard can't flood the session.
const MaxPasteBytes = 1 << 20

// SendKey types k into the attached session.
func (a *Attachment) SendKey(k tea.KeyPressMsg) {
	if a.Live() && !a.ReadOnly {
		a.snapToBottom()
		key := screen.Key{Code: k.Code, Text: k.Text, Mod: k.Mod}
		a.write(func(s AttachStream) error { return s.SendKey(key) })
	}
}

func (a *Attachment) SendPaste(text string) tea.Cmd {
	if len(text) > MaxPasteBytes {
		return notify.Warning("paste too large (over 1 MB), not sent")
	}
	if a.Live() && !a.ReadOnly {
		a.snapToBottom()
		a.write(func(s AttachStream) error { return s.Paste(text) })
	}
	return nil
}

func (a *Attachment) scroll(offset int) {
	a.offset = offset
	a.write(func(s AttachStream) error { return s.Scroll(offset) })
}

// snapToBottom returns a scrolled-back view to the live screen, so input
// typed while reading the scrollback is never sent blind.
func (a *Attachment) snapToBottom() {
	if a.offset > 0 {
		a.newLines = 0
		a.scroll(0)
	}
}

// Wheel handles the Wheel over the panel, whichever pane has focus: a child
// that reports the mouse gets it over its screen cell (x, y; inCell false off
// the screen), as with tmux, since one in the alternate screen (claude) has no
// scrollback; otherwise it scrolls the panel's view through the holder.
func (a *Attachment) Wheel(dir, x, y int, inCell bool) {
	if !a.Live() {
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

// ForwardPress sends a click over the child's screen to a child that reports
// the mouse, as the wheel is (see wheel). It reports whether the child took
// it; a click on the border or the info rows stays with gx. cell maps the
// mouse to the screen cell under it.
func (a *Attachment) ForwardPress(mouse tea.Mouse, cell func(tea.Mouse) (x, y int, ok bool)) bool {
	button, ok := mouseButton(mouse.Button)
	if !ok || !a.Live() || a.ReadOnly || !a.mouse || a.offset != 0 {
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

// ForwardMove follows a held press with the pointer, clamped to the screen so
// the child sees the drag end where it left the panel. clamp maps the mouse
// to the nearest screen cell.
func (a *Attachment) ForwardMove(mouse tea.Mouse, kind screen.MouseKind, clamp func(tea.Mouse) (x, y int)) {
	if !a.Live() || a.held == 0 {
		return
	}
	button := a.held - 1
	x, y := clamp(mouse)
	if kind == screen.MouseRelease {
		a.held = 0
	}
	a.write(func(s AttachStream) error { return s.Mouse(x, y, kind, button) })
}

// Resize gives the session the panel's size. It runs on every focus, not only
// on change: the pty follows the viewer that last focused or typed, and
// another viewer may have resized it since. Typing needs no Resize, as the
// holder claims this viewer's last size on input.
func (a *Attachment) Resize(cols, rows int) {
	if a.Live() {
		a.size = [2]int{cols, rows}
		a.write(func(s AttachStream) error { return s.Resize(cols, rows) })
	}
}

// SyncSize sends the panel's size when it changed.
func (a *Attachment) SyncSize(cols, rows int) {
	if a.Live() && a.size != [2]int{cols, rows} {
		a.Resize(cols, rows)
	}
}
