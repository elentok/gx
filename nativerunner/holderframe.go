package nativerunner

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/harness"
)

// StreamType is a holder.sock stream message's type. Types are append-only:
// a reader skips a type it does not know, so an older client keeps working
// against a newer holder.
type StreamType uint8

const (
	// StreamStatus carries the Watch's Session status and conversation id.
	StreamStatus StreamType = iota + 1
	// StreamExit carries the child's exit code; nothing follows it.
	StreamExit
	// StreamInput is a key or paste a client sends on an attach, as an
	// AttachInput. On a watch stream it says a client typed or pasted, and its
	// payload is an empty object.
	StreamInput
	// StreamFrame carries an attach's screen rows and cursor.
	StreamFrame
	// StreamError carries why the holder closes the stream; nothing follows
	// it.
	StreamError
	// StreamResize is the viewer's size a client sends on an attach when it
	// focuses, as an AttachResize.
	StreamResize
	// StreamScroll is the scroll offset a client sends on an attach, as an
	// AttachScroll.
	StreamScroll
	// StreamCopy is a client's AttachCopy request on an attach, and the
	// holder's streamCopied reply.
	StreamCopy
	// StreamWheel is a wheel turn a client sends on an attach, as an
	// AttachWheel, for a child that reports the mouse.
	StreamWheel
)

// maxStreamPayload bounds one stream message; a bigger one ends the stream
// with an error.
const maxStreamPayload = 2 << 20

// A stream message is [u32 payload length][u8 type][JSON payload], the
// length big-endian.
func writeStreamMessage(w io.Writer, typ StreamType, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(payload) > maxStreamPayload {
		return fmt.Errorf("holder stream message of %d bytes is too big", len(payload))
	}
	msg := binary.BigEndian.AppendUint32(nil, uint32(len(payload)))
	msg = append(msg, byte(typ))
	_, err = w.Write(append(msg, payload...))
	return err
}

func readStreamMessage(r io.Reader) (StreamType, []byte, error) {
	var head [5]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(head[:4])
	if n > maxStreamPayload {
		return 0, nil, fmt.Errorf("holder stream message of %d bytes is too big", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return StreamType(head[4]), payload, nil
}

// streamStatus is StreamStatus's payload.
type streamStatus struct {
	State          harness.State       `json:"state"`
	Background     bool                `json:"background,omitempty"`
	Cancelled      bool                `json:"cancelled,omitempty"`
	Blocked        harness.BlockedKind `json:"blocked,omitempty"`
	Message        string              `json:"message,omitempty"`
	ConversationID string              `json:"conversation_id,omitempty"`
	DisplayName    string              `json:"display_name,omitempty"`
}

func newStreamStatus(s harness.Status, conversationID, displayName string) streamStatus {
	return streamStatus{
		State:          s.State,
		Background:     s.Background,
		Cancelled:      s.Cancelled,
		Blocked:        s.Blocked,
		Message:        s.Message,
		ConversationID: conversationID,
		DisplayName:    displayName,
	}
}

func (s streamStatus) status() harness.Status {
	return harness.Status{State: s.State, Background: s.Background, Cancelled: s.Cancelled, Blocked: s.Blocked, Message: s.Message}
}

// streamExit is StreamExit's payload. Code is -1 for a child killed by a
// signal.
type streamExit struct {
	Code int `json:"code"`
}

// Frame is StreamFrame's payload: the cursor and the rows that changed, each
// a styled ANSI string. The first frame of an attach carries every row, and
// so does each frame of a scrolled attach, whose cursor is hidden.
type Frame struct {
	Cursor FrameCursor `json:"cursor"`
	Rows   []FrameRow  `json:"rows"`
	// Full marks a frame that carries every row of the view, so a client
	// drops any rows it kept past them.
	Full bool `json:"full,omitempty"`
	// ScrollOffset is how many lines above the screen's top the view starts;
	// ScrollMax, the scrollback's length, is the most it can be. Row i shows
	// line ScrollMax-ScrollOffset+i, the line a LinePos names.
	ScrollOffset int `json:"scroll_offset"`
	ScrollMax    int `json:"scroll_max"`
	// NewLines counts the lines output since the attach scrolled up.
	NewLines int `json:"new_lines,omitempty"`
	// Mouse is whether the child reports the mouse, so the wheel belongs to
	// it rather than to the scrollback.
	Mouse bool `json:"mouse,omitempty"`
}

type FrameCursor struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	Visible bool `json:"visible"`
}

type FrameRow struct {
	Index int    `json:"index"`
	ANSI  string `json:"ansi"`
}

// AttachInput is the payload of a StreamInput sent on an attach: a key event
// or a paste. Keys travel as events rather than bytes so the holder can encode
// them for the modes the child turned on.
type AttachInput struct {
	Key   *InputKey `json:"key,omitempty"`
	Paste string    `json:"paste,omitempty"`
}

type InputKey struct {
	Code rune          `json:"code"`
	Text string        `json:"text,omitempty"`
	Mod  screen.KeyMod `json:"mod,omitempty"`
}

// AttachResize is StreamResize's payload. The pty follows the size of the
// attach that last resized or typed; every other attach gets its frames
// cropped or padded to its own size.
type AttachResize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// AttachScroll is StreamScroll's payload. The offset is the attach's alone;
// while it is above zero, output grows it so the view stays on its lines.
type AttachScroll struct {
	Offset int `json:"offset"`
}

// AttachWheel is StreamWheel's payload: a turn down, or up, over the
// child's cell (X, Y).
type AttachWheel struct {
	X    int  `json:"x"`
	Y    int  `json:"y"`
	Down bool `json:"down,omitempty"`
}

// AttachCopy is a StreamCopy request: the plain text from Start up to, not
// including, End.
type AttachCopy struct {
	Start LinePos `json:"start"`
	End   LinePos `json:"end"`
}

// LinePos is a cell, its line counted from the oldest scrollback line, the
// screen's rows following the scrollback's.
type LinePos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// streamCopied is the holder's StreamCopy reply, each line's trailing spaces
// trimmed.
type streamCopied struct {
	Text string `json:"text"`
}

// streamError is StreamError's payload.
type streamError struct {
	Message string `json:"message"`
}
