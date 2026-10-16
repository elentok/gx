package nativerunner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/harness"
)

// HolderSocketFile is the holder's unix socket in a pty agent directory.
const HolderSocketFile = "holder.sock"

// HolderProtocolVersion is the holder.sock protocol version ping reports.
const HolderProtocolVersion = 1

// The frozen core of holder.sock ops. A holder from any gx version serves
// these unchanged, so a gx upgrade never strands a live holder; anything new
// is an optional op the client checks with Supports first.
const (
	opPing   = "ping"
	opStatus = "status"
	opScreen = "screen"
	opKeys   = "keys"
	opStop   = "stop"
)

// OpDenyReason denies a permission prompt with a reason: Esc, wait for idle,
// then the reason as a prompt, as one step.
const OpDenyReason = "deny-reason"

// OpWatch upgrades the connection to a stream of StreamStatus messages, one
// per change, ending in StreamExit. A close without StreamExit means the
// holder died.
const OpWatch = "watch"

// holderOptionalOps are the optional ops this holder serves.
var holderOptionalOps = []string{OpDenyReason, OpWatch, OpAttach}

const (
	holderIOTimeout = 5 * time.Second
	// holderStopGrace is how long stop waits for the child to exit on
	// SIGTERM before it kills it.
	holderStopGrace = 5 * time.Second
)

// holderDenyTimeout bounds each wait in deny-reason: for idle after Esc, and
// for claude to pick the reason up after it is typed.
var holderDenyTimeout = 3 * time.Second

// One request and one response per connection, each a JSON object.
type holderRequest struct {
	Op         string `json:"op"`
	Keys       string `json:"keys,omitempty"`
	Scrollback int    `json:"scrollback,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type holderResponse struct {
	Error   string   `json:"error,omitempty"`
	Version int      `json:"version,omitempty"`
	Ops     []string `json:"ops,omitempty"`
	// Pid is the child's pid, reported by ping.
	Pid        int      `json:"pid,omitempty"`
	Screen     []string `json:"screen,omitempty"`
	Scrollback []string `json:"scrollback,omitempty"`
	// Claude is nil until claude's pid status file first appears.
	Claude *PidStatus `json:"claude,omitempty"`
	Turn   int        `json:"turn,omitempty"`
}

// holderServer answers holder.sock requests for one child.
type holderServer struct {
	scr  *screen.Screen
	ptmx *os.File
	proc *os.Process
	// pid is nil for a harness without claude's pid status.
	pid     *harness.PidWatcher
	watcher harness.Watcher
	name    *displayName
	frames  *frameHub
	// inputs wakes the watch streams when a client types on an attach.
	inputs signalHub
	// changes wakes the watch streams when what they send may have changed:
	// on the watcher's own changes and on output, which drives the rest.
	changes signalHub
	ops    []string
	// exited closes once the child has exited and exitCode is set.
	exited   chan struct{}
	exitCode int
	// finished closes once the holder has saved the child's exit, so a watch
	// stream's StreamExit never beats the last-screen file.
	finished chan struct{}

	mu        sync.Mutex
	finishing bool
	// streams counts watch and attach streams still sending, which finish
	// waits for.
	streams sync.WaitGroup
}

func (h *holderServer) serve(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go h.handle(conn)
	}
}

func (h *holderServer) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(holderIOTimeout))
	var req holderRequest
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&req); err != nil {
		return
	}
	if req.Op == OpWatch && slices.Contains(h.ops, OpWatch) {
		h.watch(conn)
		return
	}
	if req.Op == OpAttach && slices.Contains(h.ops, OpAttach) {
		// The decoder may have read past the request into the first stream
		// messages.
		h.attach(conn, bufio.NewReader(io.MultiReader(dec.Buffered(), conn)))
		return
	}
	resp, after := h.answer(req)
	// deny-reason can outlast the first deadline.
	_ = conn.SetDeadline(time.Now().Add(holderIOTimeout))
	_ = json.NewEncoder(conn).Encode(resp)
	if after != nil {
		after()
	}
}

// answer returns the response to req and, for stop, the action to run once
// the response is sent.
func (h *holderServer) answer(req holderRequest) (holderResponse, func()) {
	switch req.Op {
	case opPing:
		return holderResponse{Version: HolderProtocolVersion, Ops: h.ops, Pid: h.proc.Pid}, nil
	case opStatus:
		// The pid file's last status outlives claude, and the holder keeps
		// answering while it drains, so an exit must not read as idle.
		select {
		case <-h.exited:
			return holderResponse{Error: fmt.Sprintf("claude exited (code %d)", h.exitCode)}, nil
		default:
		}
		if h.pid == nil {
			return holderResponse{}, nil
		}
		claude, turn := h.pid.PidState()
		return holderResponse{Claude: claude, Turn: turn}, nil
	case opScreen:
		return holderResponse{Screen: h.scr.Rows(), Scrollback: h.scr.ScrollbackTail(req.Scrollback)}, nil
	case opKeys:
		if _, err := h.ptmx.Write([]byte(req.Keys)); err != nil {
			return holderResponse{Error: err.Error()}, nil
		}
		return holderResponse{}, nil
	case opStop:
		return holderResponse{}, h.stop
	}
	if req.Op == OpDenyReason && slices.Contains(h.ops, OpDenyReason) {
		if err := h.denyReason(req.Reason); err != nil {
			return holderResponse{Error: err.Error()}, nil
		}
		return holderResponse{}, nil
	}
	return holderResponse{Error: fmt.Sprintf("unsupported op %q", req.Op)}, nil
}

// denyReason holds the served status at the prompt's blocked status until
// claude has taken the reason, so a status reader never sees the idle gap
// between Esc and the reason, nor the one before claude starts on it.
func (h *holderServer) denyReason(reason string) error {
	if h.pid == nil {
		return errors.New("deny-reason needs claude's pid status")
	}
	h.pid.Hold()
	defer h.pid.Release()
	if _, err := h.ptmx.Write([]byte("\x1b")); err != nil {
		return err
	}
	typedAt, ok := h.pid.WaitLive(holderDenyTimeout, func(s PidStatus, _ int) bool { return s.Status == ClaudeIdle })
	if !ok {
		return errors.New("claude did not go idle after Esc")
	}
	if _, err := h.ptmx.Write([]byte(reason)); err != nil {
		return err
	}
	// Enter apart from the text, so claude reads it as a submit rather than
	// part of a paste.
	if _, err := h.ptmx.Write([]byte("\r")); err != nil {
		return err
	}
	if _, ok := h.pid.WaitLive(holderDenyTimeout, func(_ PidStatus, turn int) bool { return turn > typedAt }); !ok {
		return errors.New("claude did not take the reason")
	}
	return nil
}

// watch streams the Watch's status until the holder finishes, then the exit.
// The status op is frozen, so this is its push-based successor.
func (h *holderServer) watch(conn net.Conn) {
	if h.startStream() {
		defer h.streams.Done()
	}
	_ = conn.SetDeadline(time.Time{})
	// The client sends nothing more; its reads end when it hangs up.
	gone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		close(gone)
	}()
	send := func(typ StreamType, v any) error {
		_ = conn.SetWriteDeadline(time.Now().Add(holderIOTimeout))
		return writeStreamMessage(conn, typ, v)
	}
	inputs := h.inputs.subscribe()
	defer h.inputs.unsubscribe(inputs)
	// Subscribed before the first read, so no change slips in between.
	changes := h.changes.subscribe()
	defer h.changes.unsubscribe(changes)
	var last *streamStatus
	for {
		cur := newStreamStatus(h.watcher.Status(), h.watcher.ConversationID(), h.name.get())
		if last == nil || cur != *last {
			if send(StreamStatus, cur) != nil {
				return
			}
			last = &cur
		}
		select {
		case <-h.finished:
			_ = send(StreamExit, streamExit{Code: h.exitCode})
			return
		case <-gone:
			return
		case <-inputs:
			if send(StreamInput, struct{}{}) != nil {
				return
			}
		case <-changes:
		}
	}
}

// signalHub wakes every subscriber without blocking; signals a subscriber
// has not taken yet collapse into one. The zero value is ready to use.
type signalHub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func (s *signalHub) subscribe() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subs == nil {
		s.subs = map[chan struct{}]struct{}{}
	}
	ch := make(chan struct{}, 1)
	s.subs[ch] = struct{}{}
	return ch
}

func (s *signalHub) unsubscribe(ch chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, ch)
}

func (s *signalHub) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Write notifies on each write, so the hub can follow the pty's output.
func (s *signalHub) Write(p []byte) (int, error) {
	s.notify()
	return len(p), nil
}

// startStream counts a new stream unless the holder is finishing, when the
// stream sends its exit at once and finish need not wait for it.
func (h *holderServer) startStream() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.finishing {
		return false
	}
	h.streams.Add(1)
	return true
}

// finish queues the last output's frame, tells the streams the exit is saved
// and waits for them to send it.
func (h *holderServer) finish() {
	h.frames.flush()
	h.mu.Lock()
	h.finishing = true
	h.mu.Unlock()
	close(h.finished)
	h.streams.Wait()
}

// stop ends the child's whole session; the holder exits once the child does.
func (h *holderServer) stop() {
	pgid := -h.proc.Pid
	_ = syscall.Kill(pgid, syscall.SIGTERM)
	select {
	case <-h.exited:
	case <-time.After(holderStopGrace):
		_ = syscall.Kill(pgid, syscall.SIGKILL)
	}
}

// HolderClient talks to a live holder over holder.sock.
type HolderClient struct {
	sock    string
	version int
	ops     []string
	pid     int
}

// DialHolder pings the holder serving dir's holder.sock.
func DialHolder(dir string) (*HolderClient, error) {
	c := &HolderClient{sock: filepath.Join(dir, HolderSocketFile)}
	resp, err := c.call(holderRequest{Op: opPing})
	if err != nil {
		return nil, err
	}
	c.version, c.ops, c.pid = resp.Version, resp.Ops, resp.Pid
	return c, nil
}

// Version is the protocol version the holder reported.
func (c *HolderClient) Version() int { return c.version }

// ChildPID is the pid of the agent the holder runs.
func (c *HolderClient) ChildPID() int { return c.pid }

// Supports reports whether the holder serves the optional op. Callers check
// it before an optional op and fall back to the core ops, or refuse just that
// action, when an older holder lacks it.
func (c *HolderClient) Supports(op string) bool { return slices.Contains(c.ops, op) }

// Status returns claude's last pid file status, nil before the file first
// appears, and the number of status changes the holder has seen.
func (c *HolderClient) Status() (*PidStatus, int, error) {
	resp, err := c.call(holderRequest{Op: opStatus})
	return resp.Claude, resp.Turn, err
}

// Screen returns the visible screen rows and up to the last scrollback rows
// that scrolled off it, oldest first.
func (c *HolderClient) Screen(scrollback int) (rows, scrolled []string, err error) {
	resp, err := c.call(holderRequest{Op: opScreen, Scrollback: scrollback})
	return resp.Screen, resp.Scrollback, err
}

// SendKeys writes keys to the child's terminal as typed input.
func (c *HolderClient) SendKeys(keys string) error {
	_, err := c.call(holderRequest{Op: opKeys, Keys: keys})
	return err
}

// Stop ends the child; the holder then records its exit and exits.
func (c *HolderClient) Stop() error {
	_, err := c.call(holderRequest{Op: opStop})
	return err
}

// DenyWithReason runs the optional deny-reason op; check Supports first.
func (c *HolderClient) DenyWithReason(reason string) error {
	_, err := c.callWithin(holderRequest{Op: OpDenyReason, Reason: reason}, holderIOTimeout+2*holderDenyTimeout)
	return err
}

// Watch opens the optional watch stream; check Supports first.
func (c *HolderClient) Watch() (*HolderWatch, error) {
	conn, err := c.openStream(OpWatch)
	if err != nil {
		return nil, err
	}
	return &HolderWatch{conn: conn}, nil
}

// openStream sends the request for a streaming op and hands back the
// connection the stream runs on.
func (c *HolderClient) openStream(op string) (net.Conn, error) {
	conn, err := net.DialTimeout("unix", c.sock, holderIOTimeout)
	if err != nil {
		return nil, err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(holderIOTimeout))
	if err := json.NewEncoder(conn).Encode(holderRequest{Op: op}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetWriteDeadline(time.Time{})
	return conn, nil
}

// HolderWatch is a live watch stream.
type HolderWatch struct {
	conn net.Conn
}

// WatchMessage is one message off a watch stream. Status, ConversationID and
// DisplayName are set for StreamStatus, ExitCode for StreamExit.
type WatchMessage struct {
	Type           StreamType
	Status         harness.Status
	ConversationID string
	// DisplayName is "" until the child sets one.
	DisplayName string
	ExitCode    int
}

// Next blocks for the next message, skipping types it does not know. An
// error before a StreamExit means the holder died.
func (w *HolderWatch) Next() (WatchMessage, error) {
	for {
		typ, payload, err := readStreamMessage(w.conn)
		if err != nil {
			return WatchMessage{}, err
		}
		switch typ {
		case StreamStatus:
			var s streamStatus
			if err := json.Unmarshal(payload, &s); err != nil {
				return WatchMessage{}, err
			}
			return WatchMessage{Type: typ, Status: s.status(), ConversationID: s.ConversationID, DisplayName: s.DisplayName}, nil
		case StreamExit:
			var e streamExit
			if err := json.Unmarshal(payload, &e); err != nil {
				return WatchMessage{}, err
			}
			return WatchMessage{Type: typ, ExitCode: e.Code}, nil
		case StreamInput:
			return WatchMessage{Type: typ}, nil
		}
	}
}

func (w *HolderWatch) Close() error { return w.conn.Close() }

func (c *HolderClient) call(req holderRequest) (holderResponse, error) {
	return c.callWithin(req, holderIOTimeout)
}

func (c *HolderClient) callWithin(req holderRequest, timeout time.Duration) (holderResponse, error) {
	var resp holderResponse
	conn, err := net.DialTimeout("unix", c.sock, holderIOTimeout)
	if err != nil {
		return resp, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return resp, err
	}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return resp, fmt.Errorf("holder %s: %w", req.Op, err)
	}
	if resp.Error != "" {
		return resp, errors.New("holder " + req.Op + ": " + resp.Error)
	}
	return resp, nil
}
