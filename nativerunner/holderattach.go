package nativerunner

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
)

// OpAttach upgrades the connection to a two-way stream: a full Frame, then a
// Frame of the rows the screen marks dirty, ending in StreamExit. The client
// runs no emulator of its own.
const OpAttach = "attach"

// ErrAttachUnsupported means the holder predates attach; fall back to the
// screen op's snapshot.
var ErrAttachUnsupported = errors.New("holder does not support attach")

// attachBuffer is how many frames an attach may fall behind before the holder
// drops it.
const attachBuffer = 64

// frameHub turns screen changes into frames for every attach. It runs on pty
// output rather than a timer, so an idle child costs nothing.
type frameHub struct {
	scr *screen.Screen

	mu     sync.Mutex
	subs   map[chan Frame]struct{}
	cursor screen.Cursor
}

func newFrameHub(scr *screen.Screen) *frameHub {
	return &frameHub{scr: scr, subs: map[chan Frame]struct{}{}}
}

// run flushes on each output signal until done closes.
func (f *frameHub) run(output <-chan struct{}, done <-chan struct{}) {
	for {
		select {
		case <-output:
			f.flush()
		case <-done:
			return
		}
	}
}

// subscribe returns a channel of the frames to come and the full frame they
// build on. It flushes first, so rows dirty before the attach are not sent to
// it again.
func (f *frameHub) subscribe() (chan Frame, Frame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushLocked()
	ch := make(chan Frame, attachBuffer)
	f.subs[ch] = struct{}{}
	rows := f.scr.StyledRows()
	all := make([]int, len(rows))
	for i := range all {
		all[i] = i
	}
	return ch, newFrame(f.scr.Cursor(), rows, all)
}

func (f *frameHub) unsubscribe(ch chan Frame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.subs, ch)
}

// flush sends the dirty rows to every attach, closing the channel of one
// that has fallen too far behind.
func (f *frameHub) flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushLocked()
}

func (f *frameHub) flushLocked() {
	dirty := f.scr.TakeDirty()
	cur := f.scr.Cursor()
	moved := cur != f.cursor
	// Kept even with no attach, as the cursor a new attach's frame starts at.
	f.cursor = cur
	if len(f.subs) == 0 || len(dirty) == 0 && !moved {
		return
	}
	frame := newFrame(cur, f.scr.StyledRows(), dirty)
	for ch := range f.subs {
		select {
		case ch <- frame:
		default:
			close(ch)
			delete(f.subs, ch)
		}
	}
}

func newFrame(cur screen.Cursor, rows []string, indexes []int) Frame {
	frame := Frame{Cursor: FrameCursor{X: cur.X, Y: cur.Y, Visible: cur.Visible}}
	for _, i := range indexes {
		if i < len(rows) {
			frame.Rows = append(frame.Rows, FrameRow{Index: i, ANSI: rows[i]})
		}
	}
	return frame
}

// outputSignal wakes the frameHub after each pty write without ever blocking
// the pty reader; signals that pile up collapse into one.
type outputSignal chan struct{}

func (s outputSignal) Write(p []byte) (int, error) {
	select {
	case s <- struct{}{}:
	default:
	}
	return len(p), nil
}

// attach streams frames until the holder finishes, then the exit.
func (h *holderServer) attach(conn net.Conn) {
	if h.startStream() {
		defer h.streams.Done()
	}
	_ = conn.SetDeadline(time.Time{})
	frames, full := h.frames.subscribe()
	defer h.frames.unsubscribe(frames)
	// Client messages have no use yet beyond the size check; unknown types
	// are skipped so newer clients keep working.
	clientErr := make(chan error, 1)
	go func() {
		for {
			if _, _, err := readStreamMessage(conn); err != nil {
				clientErr <- err
				return
			}
		}
	}()
	send := func(typ StreamType, v any) error {
		_ = conn.SetWriteDeadline(time.Now().Add(holderIOTimeout))
		return writeStreamMessage(conn, typ, v)
	}
	if send(StreamFrame, full) != nil {
		return
	}
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				_ = send(StreamError, streamError{Message: "attach fell behind"})
				return
			}
			if send(StreamFrame, frame) != nil {
				return
			}
		case <-h.finished:
			// finish flushed the last output before closing finished.
			if sendPending(frames, send) != nil {
				return
			}
			_ = send(StreamExit, streamExit{Code: h.exitCode})
			return
		case err := <-clientErr:
			if !errors.Is(err, io.EOF) {
				_ = send(StreamError, streamError{Message: err.Error()})
			}
			return
		}
	}
}

// sendPending sends the frames already queued, without waiting for more.
func sendPending(frames chan Frame, send func(StreamType, any) error) error {
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				return errors.New("attach fell behind")
			}
			if err := send(StreamFrame, frame); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

// Attach opens the optional attach stream, or returns ErrAttachUnsupported
// for an older holder.
func (c *HolderClient) Attach() (*HolderAttach, error) {
	if !c.Supports(OpAttach) {
		return nil, ErrAttachUnsupported
	}
	conn, err := c.openStream(OpAttach)
	if err != nil {
		return nil, err
	}
	return &HolderAttach{conn: conn}, nil
}

// HolderAttach is a live attach stream.
type HolderAttach struct {
	conn net.Conn
	// writeMu keeps concurrent Sends from interleaving their bytes.
	writeMu sync.Mutex
}

// AttachMessage is one message off an attach stream. Frame is set for
// StreamFrame, ExitCode for StreamExit.
type AttachMessage struct {
	Type     StreamType
	Frame    Frame
	ExitCode int
}

// Next blocks for the next message, skipping types it does not know. A
// StreamError comes back as an error; an error before a StreamExit means the
// holder died or dropped the attach.
func (a *HolderAttach) Next() (AttachMessage, error) {
	for {
		typ, payload, err := readStreamMessage(a.conn)
		if err != nil {
			return AttachMessage{}, err
		}
		switch typ {
		case StreamFrame:
			var f Frame
			if err := json.Unmarshal(payload, &f); err != nil {
				return AttachMessage{}, err
			}
			return AttachMessage{Type: typ, Frame: f}, nil
		case StreamExit:
			var e streamExit
			if err := json.Unmarshal(payload, &e); err != nil {
				return AttachMessage{}, err
			}
			return AttachMessage{Type: typ, ExitCode: e.Code}, nil
		case StreamError:
			var e streamError
			if err := json.Unmarshal(payload, &e); err != nil {
				return AttachMessage{}, err
			}
			return AttachMessage{}, errors.New("holder attach: " + e.Message)
		}
	}
}

// Send writes a message to the holder.
func (a *HolderAttach) Send(typ StreamType, v any) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	_ = a.conn.SetWriteDeadline(time.Now().Add(holderIOTimeout))
	return writeStreamMessage(a.conn, typ, v)
}

func (a *HolderAttach) Close() error { return a.conn.Close() }
