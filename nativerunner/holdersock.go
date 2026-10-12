package nativerunner

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
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

// holderOptionalOps are the optional ops this holder serves.
var holderOptionalOps = []string{OpDenyReason}

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
	pid  *pidWatcher
	ops  []string
	// exited closes once the child has exited.
	exited chan struct{}
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
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
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
		claude, turn := h.pid.state()
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
	h.pid.hold()
	defer h.pid.release()
	if _, err := h.ptmx.Write([]byte("\x1b")); err != nil {
		return err
	}
	typedAt, ok := h.pid.waitLive(holderDenyTimeout, func(s PidStatus, _ int) bool { return s.Status == ClaudeIdle })
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
	if _, ok := h.pid.waitLive(holderDenyTimeout, func(_ PidStatus, turn int) bool { return turn > typedAt }); !ok {
		return errors.New("claude did not take the reason")
	}
	return nil
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
