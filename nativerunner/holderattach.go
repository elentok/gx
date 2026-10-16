package nativerunner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/agentrunner/screen"
)

// OpAttach upgrades the connection to a two-way stream: a full Frame, then a
// Frame of the rows the screen marks dirty, ending in StreamExit. The client
// runs no emulator of its own.
const OpAttach = "attach"

// ErrAttachUnsupported means the holder predates attach; fall back to the
// screen op's snapshot.
var ErrAttachUnsupported = errors.New("holder does not support attach")

// An attach gets at most one frame per frameInterval, except within
// keyEchoWindow of its own client's key, so typing echoes at once while a
// burst of output still costs only a frame per display refresh.
const (
	frameInterval = 16 * time.Millisecond
	keyEchoWindow = 32 * time.Millisecond
)

// armedFrameTimers counts the pacing timers armed across all attaches.
var armedFrameTimers atomic.Int64

// frameHub turns screen changes into frames for every attach. It runs on pty
// output rather than a ticker, and arms a timer only to hold back output that
// came too soon, so an idle child costs nothing.
type frameHub struct {
	scr *screen.Screen
	// setSize resizes the pty and the screen.
	setSize func(cols, rows int) error

	mu     sync.Mutex
	subs   map[*attachClient]struct{}
	cursor screen.Cursor
	// cols and rows are the pty's size, which outlives the attaches.
	cols, rows int
}

// attachClient is one attach's pacing state, guarded by frameHub.mu.
type attachClient struct {
	// slot holds the latest frame the attach has yet to send. Only the hub
	// fills it, so a slow client never blocks the hub, only misses frames.
	slot chan Frame
	// rows and moved are what changed since the last frame.
	rows  map[int]struct{}
	moved bool
	// full means a frame was dropped, so the next one must carry every row.
	full  bool
	last  time.Time
	keyed time.Time
	timer *time.Timer
	// width and height are the viewer's size, zero until it resizes, when its
	// frames are the screen as is.
	width, height int
}

func (c *attachClient) pending() bool {
	return len(c.rows) > 0 || c.moved || c.full
}

func newFrameHub(scr *screen.Screen, cols, rows int, setSize func(cols, rows int) error) *frameHub {
	return &frameHub{scr: scr, setSize: setSize, cols: cols, rows: rows, subs: map[*attachClient]struct{}{}}
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

// subscribe returns a client whose slot gets the frames to come and the full
// frame they build on. It flushes first, so rows dirty before the attach are
// not sent to it again.
func (f *frameHub) subscribe() (*attachClient, Frame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushLocked()
	c := &attachClient{slot: make(chan Frame, 1), rows: map[int]struct{}{}, last: time.Now()}
	f.subs[c] = struct{}{}
	return c, f.fullFrameLocked(c)
}

func (f *frameHub) unsubscribe(c *attachClient) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopTimerLocked(c)
	delete(f.subs, c)
}

// flush hands the dirty rows to every attach.
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
	if len(dirty) == 0 && !moved {
		return
	}
	for c := range f.subs {
		for _, i := range dirty {
			c.rows[i] = struct{}{}
		}
		c.moved = c.moved || moved
		f.paceLocked(c)
	}
}

// keyed opens c's key echo window, sending what its timer holds back at once.
func (f *frameHub) keyed(c *attachClient) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c.keyed = time.Now()
	f.paceLocked(c)
}

// resized records c's viewer size and gives c the pty's size.
func (f *frameHub) resized(c *attachClient, width, height int) error {
	if width <= 0 || height <= 0 || width > math.MaxUint16 || height > math.MaxUint16 {
		return fmt.Errorf("invalid attach size %dx%d", width, height)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c.width, c.height = width, height
	return f.ownLocked(c)
}

// own gives c the pty's size, as the viewer that last focused or typed.
func (f *frameHub) own(c *attachClient) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ownLocked(c)
}

// ownLocked resizes the pty to c's size, and since every attach's view of the
// screen changes with it, owes each one a full frame.
func (f *frameHub) ownLocked(c *attachClient) error {
	if c.width == 0 || (c.width == f.cols && c.height == f.rows) {
		return nil
	}
	if err := f.setSize(c.width, c.height); err != nil {
		return err
	}
	f.cols, f.rows = c.width, c.height
	// The full frames cover the rows the resize dirtied.
	f.scr.TakeDirty()
	f.cursor = f.scr.Cursor()
	for s := range f.subs {
		clear(s.rows)
		s.moved = false
		s.full = true
		f.paceLocked(s)
	}
	return nil
}

// sent tells the hub c's slot is free, so the full frame owed for a drop goes
// out without waiting for more output.
func (f *frameHub) sent(c *attachClient) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paceLocked(c)
}

// drain returns c's last frames, ignoring the pacing: the frame in its slot,
// then any it is still owed.
func (f *frameHub) drain(c *attachClient) []Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopTimerLocked(c)
	var frames []Frame
	select {
	case frame := <-c.slot:
		frames = append(frames, frame)
	default:
	}
	if c.pending() {
		frames = append(frames, f.takeFrameLocked(c))
	}
	return frames
}

// paceLocked puts c's pending changes in its slot now, or arms a timer for
// when its pacing allows.
func (f *frameHub) paceLocked(c *attachClient) {
	if !c.pending() {
		return
	}
	now := time.Now()
	wait := c.last.Add(frameInterval).Sub(now)
	if now.Sub(c.keyed) < keyEchoWindow {
		wait = 0
	}
	if wait > 0 {
		if c.timer == nil {
			f.armTimerLocked(c, wait)
		}
		return
	}
	f.stopTimerLocked(c)
	if len(c.slot) == cap(c.slot) {
		clear(c.rows)
		c.moved = false
		c.full = true
		return
	}
	c.slot <- f.takeFrameLocked(c)
	c.last = now
}

func (f *frameHub) armTimerLocked(c *attachClient, wait time.Duration) {
	armedFrameTimers.Add(1)
	var t *time.Timer
	t = time.AfterFunc(wait, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		// A timer stopped too late to cancel still runs; it finds itself
		// replaced.
		if c.timer != t {
			return
		}
		c.timer = nil
		armedFrameTimers.Add(-1)
		f.paceLocked(c)
	})
	c.timer = t
}

func (f *frameHub) stopTimerLocked(c *attachClient) {
	if c.timer == nil {
		return
	}
	c.timer.Stop()
	c.timer = nil
	armedFrameTimers.Add(-1)
}

// takeFrameLocked builds c's pending frame and marks it sent.
func (f *frameHub) takeFrameLocked(c *attachClient) Frame {
	var frame Frame
	if c.full {
		frame = f.fullFrameLocked(c)
	} else {
		frame = c.frame(f.cursor, f.scr.StyledRows(), slices.Sorted(maps.Keys(c.rows)))
	}
	clear(c.rows)
	c.moved = false
	c.full = false
	return frame
}

func (f *frameHub) fullFrameLocked(c *attachClient) Frame {
	rows := f.scr.StyledRows()
	n := len(rows)
	if c.height > 0 {
		n = c.height
	}
	all := make([]int, n)
	for i := range all {
		all[i] = i
	}
	return c.frame(f.cursor, rows, all)
}

// frame builds a frame of the screen's rows at indexes, cropped or padded to
// c's viewer size.
func (c *attachClient) frame(cur screen.Cursor, rows []string, indexes []int) Frame {
	frame := Frame{Cursor: FrameCursor{X: cur.X, Y: cur.Y, Visible: cur.Visible}}
	if c.width == 0 {
		for _, i := range indexes {
			if i < len(rows) {
				frame.Rows = append(frame.Rows, FrameRow{Index: i, ANSI: rows[i]})
			}
		}
		return frame
	}
	if cur.X >= c.width || cur.Y >= c.height {
		frame.Cursor.Visible = false
	}
	for _, i := range indexes {
		if i >= c.height {
			continue
		}
		var row string
		if i < len(rows) {
			row = rows[i]
		}
		frame.Rows = append(frame.Rows, FrameRow{Index: i, ANSI: fitRow(row, c.width)})
	}
	return frame
}

func fitRow(row string, width int) string {
	w := ansi.StringWidth(row)
	if w > width {
		return ansi.Truncate(row, width, "")
	}
	// The reset keeps a style left open at the row's end off the padding.
	if w < width {
		return row + "\x1b[m" + strings.Repeat(" ", width-w)
	}
	return row
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

// attach streams frames until the holder finishes, then the exit, and reads
// the client's messages from in.
func (h *holderServer) attach(conn net.Conn, in *bufio.Reader) {
	if h.startStream() {
		defer h.streams.Done()
	}
	_ = conn.SetDeadline(time.Time{})
	client, full := h.frames.subscribe()
	defer h.frames.unsubscribe(client)
	// Unknown client message types are skipped so newer clients keep working.
	clientErr := make(chan error, 1)
	go func() {
		// The request's JSON ends in a newline. A stream header never starts
		// with one, its length being capped well below 0x0a000000.
		if b, err := in.Peek(1); err == nil && b[0] == '\n' {
			_, _ = in.Discard(1)
		}
		for {
			typ, payload, err := readStreamMessage(in)
			if err == nil {
				switch typ {
				case StreamInput:
					// The child gets the key at the typing viewer's size.
					if err = h.frames.own(client); err == nil {
						if err = h.input(payload); err == nil {
							h.frames.keyed(client)
						}
					}
				case StreamResize:
					var size AttachResize
					if err = json.Unmarshal(payload, &size); err == nil {
						err = h.frames.resized(client, size.Cols, size.Rows)
					}
				}
			}
			if err != nil {
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
		case frame := <-client.slot:
			if send(StreamFrame, frame) != nil {
				return
			}
			h.frames.sent(client)
		case <-h.finished:
			// finish flushed the last output before closing finished.
			for _, frame := range h.frames.drain(client) {
				if send(StreamFrame, frame) != nil {
					return
				}
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

// input encodes a key or paste for the child's modes, which the screen writes
// to the pty, and tells every watch stream a client typed.
func (h *holderServer) input(payload []byte) error {
	var in AttachInput
	if err := json.Unmarshal(payload, &in); err != nil {
		return err
	}
	switch {
	case in.Key != nil:
		h.scr.SendKey(screen.Key{Code: in.Key.Code, Text: in.Key.Text, Mod: in.Key.Mod})
	case in.Paste != "":
		h.scr.Paste(in.Paste)
	default:
		return nil
	}
	h.inputs.notify()
	return nil
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

// SendKey types k into the child.
func (a *HolderAttach) SendKey(k screen.Key) error {
	return a.Send(StreamInput, AttachInput{Key: &InputKey{Code: k.Code, Text: k.Text, Mod: k.Mod}})
}

// Paste pastes text into the child.
func (a *HolderAttach) Paste(text string) error {
	return a.Send(StreamInput, AttachInput{Paste: text})
}

// Resize sends the viewer's size, which the child takes on, as the viewer
// that last focused. Send it on focus.
func (a *HolderAttach) Resize(cols, rows int) error {
	return a.Send(StreamResize, AttachResize{Cols: cols, Rows: rows})
}

// Send writes a message to the holder.
func (a *HolderAttach) Send(typ StreamType, v any) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	_ = a.conn.SetWriteDeadline(time.Now().Add(holderIOTimeout))
	return writeStreamMessage(a.conn, typ, v)
}

func (a *HolderAttach) Close() error { return a.conn.Close() }
