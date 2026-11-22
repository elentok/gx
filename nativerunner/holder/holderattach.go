package holder

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
const defaultFrameInterval = 16 * time.Millisecond

var (
	frameInterval = defaultFrameInterval
	keyEchoWindow = 2 * defaultFrameInterval
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
	// pushed is the screen's Pushed as of the last flush.
	pushed int
	// mouse is the screen's MouseReporting as of the last flush.
	mouse bool
	// resizedSinceOutput is set by a pty resize and cleared by the child's
	// next output: until then the screen is what the child drew for the old
	// size, so a read of it can't be trusted.
	resizedSinceOutput bool
	// owner is the attach whose keys and size count; nil means gx owns.
	owner *attachClient
	// held is set while gx holds the input: from launch with HoldInput until
	// the first batch ends, and during each batch. Take-overs are ignored and a
	// lone attach does not become the owner.
	held bool
	// batch is set while a gx batch is open; launchHold is the launch hold.
	batch, launchHold bool
	// handBack is the attach that owns when the hold ends: the owner a batch
	// took the input from, or a lone attach that came during the hold.
	handBack *attachClient
	// batchTimer releases a batch gx never ended; batchGen tells a stale timer
	// from the current one.
	batchTimer *time.Timer
	batchGen   int
}

// batchTimeout is how long a batch holds the input after gx's last key in it
// before the holder releases it, for a gx that died mid-batch.
var batchTimeout = 15 * time.Second

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
	// offset is the viewer's scroll offset; newLines counts the lines output
	// since it left zero.
	offset, newLines int
	// ownerChanged wakes the attach loop to report the owner; the loop reads
	// the current owner, so wakes coalesce.
	ownerChanged chan struct{}
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
			f.mu.Lock()
			f.resizedSinceOutput = false
			f.flushLocked()
			f.mu.Unlock()
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
	c := &attachClient{slot: make(chan Frame, 1), rows: map[int]struct{}{}, last: time.Now(), ownerChanged: make(chan struct{}, 1)}
	lone := len(f.subs) == 0
	f.subs[c] = struct{}{}
	if lone {
		if f.held {
			f.handBack = c
		} else {
			// Reported with the attach's first messages, not by a wake.
			f.owner = c
		}
	}
	return c, f.fullFrameLocked(c)
}

// holdLaunch makes gx own the input from launch until the first batch ends.
func (f *frameHub) holdLaunch() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.launchHold = true
	f.held = true
}

// beginBatch makes gx the owner until endBatch, remembering the owner to hand
// back to. A second begin only restarts the safety timer.
func (f *frameHub) beginBatch() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.batch {
		f.batch, f.held = true, true
		if f.owner != nil {
			f.handBack = f.owner
		}
		f.setOwnerLocked(nil)
	}
	f.armBatchLocked()
}

// gxKeyed restarts the batch's safety timer; gx wrote to the child.
func (f *frameHub) gxKeyed() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.batch {
		f.armBatchLocked()
	}
}

func (f *frameHub) armBatchLocked() {
	if f.batchTimer != nil {
		f.batchTimer.Stop()
	}
	f.batchGen++
	gen := f.batchGen
	f.batchTimer = time.AfterFunc(batchTimeout, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.batch && f.batchGen == gen {
			_ = f.endBatchLocked()
		}
	})
}

// endBatch returns the input to the attach it came from, which the pty then
// takes the size of. Gx keeps it when that attach left. The first end also
// ends the launch hold.
func (f *frameHub) endBatch() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.endBatchLocked()
}

func (f *frameHub) endBatchLocked() error {
	if f.batchTimer != nil {
		f.batchTimer.Stop()
		f.batchTimer = nil
	}
	f.batchGen++
	f.batch, f.launchHold, f.held = false, false, false
	c := f.handBack
	f.handBack = nil
	if c == nil {
		return nil
	}
	f.setOwnerLocked(c)
	return f.ownLocked(c)
}

// unsubscribe drops c. If c owned the session, gx does, and the remaining
// attaches are not promoted.
func (f *frameHub) unsubscribe(c *attachClient) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopTimerLocked(c)
	delete(f.subs, c)
	if f.handBack == c {
		f.handBack = nil
	}
	if f.owner == c {
		f.setOwnerLocked(nil)
	}
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
	pushed := f.scr.Pushed()
	grown := pushed - f.pushed
	f.pushed = pushed
	// A mouse mode change goes out with the cursor, as frames carry both.
	if mouse := f.scr.MouseReporting(); mouse != f.mouse {
		f.mouse, moved = mouse, true
	}
	if len(dirty) == 0 && !moved && grown == 0 {
		return
	}
	for c := range f.subs {
		if c.offset > 0 {
			// A scrolled view changes only when lines arrive or it reaches
			// down into the screen's dirty rows.
			if grown > 0 || (len(dirty) > 0 && c.offset < f.viewHeightLocked(c)) {
				c.offset += grown
				c.newLines += grown
				c.full = true
			}
		} else {
			for _, i := range dirty {
				c.rows[i] = struct{}{}
			}
			c.moved = c.moved || moved
		}
		f.paceLocked(c)
	}
}

func (f *frameHub) viewHeightLocked(c *attachClient) int {
	if c.height > 0 {
		return c.height
	}
	return f.rows
}

// scroll sets c's scroll offset and sends its new view at once.
func (f *frameHub) scroll(c *attachClient, offset int) error {
	if offset < 0 {
		return fmt.Errorf("invalid scroll offset %d", offset)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c.offset = min(offset, f.scr.ScrollbackLen())
	if c.offset == 0 {
		c.newLines = 0
	}
	clear(c.rows)
	c.moved = false
	c.full = true
	c.keyed = time.Now()
	f.paceLocked(c)
	return nil
}

// keyed opens c's key echo window, sending what its timer holds back at once.
func (f *frameHub) keyed(c *attachClient) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c.keyed = time.Now()
	f.paceLocked(c)
}

// resized records c's viewer size. Only the owner's size reaches the pty; a
// non-owner's changes its own view alone.
func (f *frameHub) resized(c *attachClient, width, height int) error {
	if width <= 0 || height <= 0 || width > math.MaxUint16 || height > math.MaxUint16 {
		return fmt.Errorf("invalid attach size %dx%d", width, height)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c.width, c.height = width, height
	if f.owner != c {
		c.full = true
		f.paceLocked(c)
		return nil
	}
	return f.ownLocked(c)
}

// takeOver makes c the owner, so its keys reach the child and the pty takes
// its size. Focus and typing never call it.
func (f *frameHub) takeOver(c *attachClient) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.held {
		return nil
	}
	f.setOwnerLocked(c)
	return f.ownLocked(c)
}

// isOwner reports whether c's keys reach the child.
func (f *frameHub) isOwner(c *attachClient) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.owner == c
}

// gxOwns reports whether no attach owns the session.
func (f *frameHub) gxOwns() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.owner == nil
}

// setOwnerLocked moves the owner, nil for gx, and tells every attach.
func (f *frameHub) setOwnerLocked(owner *attachClient) {
	if f.owner == owner {
		return
	}
	f.owner = owner
	for s := range f.subs {
		select {
		case s.ownerChanged <- struct{}{}:
		default:
		}
	}
}

// ownLocked resizes the pty to c's size, and since every attach's view of the
// screen changes with it, owes each one a full frame.
func (f *frameHub) ownLocked(c *attachClient) error {
	if c.width == 0 || (c.width == f.cols && c.height == f.rows) {
		return nil
	}
	return f.applySizeLocked(c.width, c.height)
}

// applySizeLocked resizes the pty and the screen and owes every attach a full
// frame.
func (f *frameHub) applySizeLocked(width, height int) error {
	if err := f.setSize(width, height); err != nil {
		return err
	}
	f.cols, f.rows = width, height
	f.resizedSinceOutput = true
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

// nudge resizes the pty by one column, alternating up and down so repeated
// calls stay in range. It is for the test hook that resizes before a read.
func (f *frameHub) nudge() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cols%2 == 0 {
		return f.applySizeLocked(f.cols+1, f.rows)
	}
	return f.applySizeLocked(f.cols-1, f.rows)
}

// ReadState is what a styled screen read says about the sizes behind it.
type ReadState struct {
	// Resized means the pty was resized and the child has not output since.
	Resized bool `json:"resized,omitempty"`
	// PTY is the pty's size; Attaches are the sizes of the attached viewers
	// that have one.
	PTY      [2]int   `json:"pty,omitempty"`
	Attaches [][2]int `json:"attaches,omitempty"`
}

// readState reports the sizes behind a screen read.
func (f *frameHub) readState() ReadState {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := ReadState{Resized: f.resizedSinceOutput, PTY: [2]int{f.cols, f.rows}}
	for c := range f.subs {
		if c.width > 0 {
			st.Attaches = append(st.Attaches, [2]int{c.width, c.height})
		}
	}
	return st
}

// sent tells the hub c's slot is free, so the full frame owed for a drop goes
// out without waiting for more output.
func (f *frameHub) sent(c *attachClient) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paceLocked(c)
}

// takeRemaining returns c's last frames, ignoring the pacing: the frame in its slot,
// then any it is still owed.
func (f *frameHub) takeRemaining(c *attachClient) []Frame {
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
	if c.full || c.offset > 0 {
		frame = f.fullFrameLocked(c)
	} else {
		frame = c.frame(f.cursor, f.scr.StyledRows(), slices.Sorted(maps.Keys(c.rows)))
		frame.ScrollBase, frame.ScrollMax = f.scr.ScrollbackSpan()
	}
	frame.Mouse = f.mouse
	clear(c.rows)
	c.moved = false
	c.full = false
	return frame
}

func (f *frameHub) fullFrameLocked(c *attachClient) Frame {
	var frame Frame
	if c.offset > 0 {
		frame = f.scrolledFrameLocked(c)
	} else {
		rows := f.scr.StyledRows()
		n := len(rows)
		if c.height > 0 {
			n = c.height
		}
		frame = c.frame(f.cursor, rows, allIndexes(n))
		frame.ScrollBase, frame.ScrollMax = f.scr.ScrollbackSpan()
	}
	frame.Full = true
	frame.Mouse = f.mouse
	return frame
}

// scrolledFrameLocked builds c's view of the scrollback and screen at its
// offset, which the scrollback shrinking may lower.
func (f *frameHub) scrolledFrameLocked(c *attachClient) Frame {
	n := f.viewHeightLocked(c)
	rows, trimmed, scrollMax := f.scr.StyledView(c.offset, n)
	c.offset = min(c.offset, scrollMax)
	frame := c.frame(f.cursor, rows, allIndexes(n))
	frame.Cursor.Visible = false
	frame.ScrollOffset = c.offset
	frame.ScrollMax = scrollMax
	frame.ScrollBase = trimmed
	frame.NewLines = c.newLines
	return frame
}

func allIndexes(n int) []int {
	all := make([]int, n)
	for i := range all {
		all[i] = i
	}
	return all
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
	// Copy replies go out through the loop below, the conn's only writer.
	copied := make(chan streamCopied)
	blocked := make(chan struct{}, 1)
	done := make(chan struct{})
	defer close(done)
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
					if !h.frames.isOwner(client) {
						// Dropped, never queued; one report covers a burst.
						select {
						case blocked <- struct{}{}:
						default:
						}
					} else if err = h.input(payload); err == nil {
						h.frames.keyed(client)
					}
				case StreamTakeOver:
					err = h.frames.takeOver(client)
				case StreamResize:
					var size AttachResize
					if err = json.Unmarshal(payload, &size); err == nil {
						err = h.frames.resized(client, size.Cols, size.Rows)
					}
				case StreamScroll:
					var s AttachScroll
					if err = json.Unmarshal(payload, &s); err == nil {
						err = h.frames.scroll(client, s.Offset)
					}
				case StreamWheel:
					var w AttachWheel
					if err = json.Unmarshal(payload, &w); err == nil {
						h.scr.SendWheel(w.X, w.Y, w.Down)
					}
				case StreamMouse:
					var m AttachMouse
					if err = json.Unmarshal(payload, &m); err == nil {
						h.scr.SendMouse(m.X, m.Y, m.Kind, m.Button)
					}
				case StreamCopy:
					var c AttachCopy
					if err = json.Unmarshal(payload, &c); err == nil {
						select {
						case copied <- streamCopied{Text: h.scr.Text(c.Start.Line, c.Start.Col, c.End.Line, c.End.Col)}:
						case <-done:
							return
						}
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
	sendOwner := func() error {
		owned := h.frames.isOwner(client)
		return send(StreamOwner, AttachOwner{Self: owned, GX: !owned && h.frames.gxOwns()})
	}
	if sendOwner() != nil {
		return
	}
	for {
		select {
		case <-client.ownerChanged:
			if sendOwner() != nil {
				return
			}
		case <-blocked:
			if send(StreamBlocked, struct{}{}) != nil {
				return
			}
		case frame := <-client.slot:
			if send(StreamFrame, frame) != nil {
				return
			}
			h.frames.sent(client)
		case c := <-copied:
			if send(StreamCopy, c) != nil {
				return
			}
		case <-h.finished:
			// finish flushed the last output before closing finished.
			for _, frame := range h.frames.takeRemaining(client) {
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
// StreamFrame, ExitCode for StreamExit, Text for StreamCopy, Owner for
// StreamOwner. StreamBlocked has no fields.
type AttachMessage struct {
	Type     StreamType
	Frame    Frame
	ExitCode int
	Text     string
	Owner    AttachOwner
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
		case StreamCopy:
			var c streamCopied
			if err := json.Unmarshal(payload, &c); err != nil {
				return AttachMessage{}, err
			}
			return AttachMessage{Type: typ, Text: c.Text}, nil
		case StreamOwner:
			var o AttachOwner
			if err := json.Unmarshal(payload, &o); err != nil {
				return AttachMessage{}, err
			}
			return AttachMessage{Type: typ, Owner: o}, nil
		case StreamBlocked:
			return AttachMessage{Type: typ}, nil
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

// TakeOver asks to become the owner, whose keys reach the child and whose size
// the child takes on.
func (a *HolderAttach) TakeOver() error {
	return a.Send(StreamTakeOver, struct{}{})
}

// Resize sends the viewer's size. The child takes it on only while this
// attach owns the session.
func (a *HolderAttach) Resize(cols, rows int) error {
	return a.Send(StreamResize, AttachResize{Cols: cols, Rows: rows})
}

// Scroll sets this attach's scroll offset, in lines above the screen's top.
func (a *HolderAttach) Scroll(offset int) error {
	return a.Send(StreamScroll, AttachScroll{Offset: offset})
}

// Wheel turns the wheel down, or up, over the child's cell (x, y), for a
// child whose frames say it reports the mouse.
func (a *HolderAttach) Wheel(x, y int, down bool) error {
	return a.Send(StreamWheel, AttachWheel{X: x, Y: y, Down: down})
}

// Mouse sends a press, release or drag of a button over the child's cell
// (x, y), for a child whose frames say it reports the mouse.
func (a *HolderAttach) Mouse(x, y int, kind screen.MouseKind, button int) error {
	return a.Send(StreamMouse, AttachMouse{X: x, Y: y, Kind: kind, Button: button})
}

// Copy asks for the plain text from start up to end; Next returns it as a
// StreamCopy message.
func (a *HolderAttach) Copy(start, end LinePos) error {
	return a.Send(StreamCopy, AttachCopy{Start: start, End: end})
}

// Send writes a message to the holder.
func (a *HolderAttach) Send(typ StreamType, v any) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	_ = a.conn.SetWriteDeadline(time.Now().Add(holderIOTimeout))
	return writeStreamMessage(a.conn, typ, v)
}

func (a *HolderAttach) Close() error { return a.conn.Close() }
