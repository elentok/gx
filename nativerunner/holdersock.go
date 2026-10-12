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

// holderOptionalOps are the optional ops this holder serves.
var holderOptionalOps []string

const (
	holderIOTimeout = 5 * time.Second
	// holderStopGrace is how long stop waits for the child to exit on
	// SIGTERM before it kills it.
	holderStopGrace = 5 * time.Second
)

// One request and one response per connection, each a JSON object.
type holderRequest struct {
	Op         string `json:"op"`
	Keys       string `json:"keys,omitempty"`
	Scrollback int    `json:"scrollback,omitempty"`
}

type holderResponse struct {
	Error      string   `json:"error,omitempty"`
	Version    int      `json:"version,omitempty"`
	Ops        []string `json:"ops,omitempty"`
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
		return holderResponse{Version: HolderProtocolVersion, Ops: h.ops}, nil
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
	return holderResponse{Error: fmt.Sprintf("unsupported op %q", req.Op)}, nil
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
}

// DialHolder pings the holder serving dir's holder.sock.
func DialHolder(dir string) (*HolderClient, error) {
	c := &HolderClient{sock: filepath.Join(dir, HolderSocketFile)}
	resp, err := c.call(holderRequest{Op: opPing})
	if err != nil {
		return nil, err
	}
	c.version, c.ops = resp.Version, resp.Ops
	return c, nil
}

// Version is the protocol version the holder reported.
func (c *HolderClient) Version() int { return c.version }

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

func (c *HolderClient) call(req holderRequest) (holderResponse, error) {
	var resp holderResponse
	conn, err := net.DialTimeout("unix", c.sock, holderIOTimeout)
	if err != nil {
		return resp, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(holderIOTimeout))
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
