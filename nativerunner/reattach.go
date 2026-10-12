package nativerunner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
)

// Verdict is what Reattach found in an agent directory.
type Verdict int

const (
	// VerdictLive: claude is still running and the session is adopted.
	VerdictLive Verdict = iota + 1
	// VerdictFinished: claude is gone but its last turn ran to a result,
	// so the iteration counts as a normal finish.
	VerdictFinished
	// VerdictDied: claude is gone mid-turn. The caller parks the ticket
	// needs-repair with ReasonAgentDied and keeps the directory. gx never
	// resumes it on its own, so no work is repeated by surprise.
	VerdictDied
)

// ReasonAgentDied is the needs-repair reason for VerdictDied.
const ReasonAgentDied = "agent-died-while-server-down"

// DefaultLogRetention is how long Prune keeps an agent directory.
const DefaultLogRetention = 30 * 24 * time.Hour

// Reattach finds the agent a previous server launched under label. A live
// one is adopted, so Find, Status and the other verbs work on it again, and
// its state is rebuilt by replaying out.jsonl.
func (h *Headless) Reattach(label string) (agentrunner.Session, Verdict, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ss, ok := h.sessions[label]; ok {
		return ss.session, VerdictLive, nil
	}
	dir := filepath.Join(h.Root, label)
	meta, err := readMeta(dir)
	if err != nil {
		return agentrunner.Session{}, 0, err
	}
	live, err := h.alive(meta)
	if err != nil {
		return agentrunner.Session{}, 0, err
	}
	// Replay before returning, so the caller sees the rebuilt state at once.
	out := filepath.Join(dir, OutFile)
	track := tracker{sessionID: meta.SessionID}
	offset, err := replay(out, meta.Offset, &track)
	if err != nil {
		return agentrunner.Session{}, 0, err
	}
	if !live {
		if track.ended() {
			return agentrunner.Session{}, VerdictFinished, nil
		}
		return agentrunner.Session{}, VerdictDied, nil
	}

	stdin, err := os.OpenFile(filepath.Join(dir, StdinFile), os.O_RDWR, 0)
	if err != nil {
		return agentrunner.Session{}, 0, err
	}
	ss := newSession(label, meta.Epic, meta.SessionID, meta.PID, stdin)
	ss.track = track
	go h.watchExit(ss, meta)
	h.follow(ss, out, offset)
	return ss.session, VerdictLive, nil
}

func (h *Headless) alive(meta Meta) (bool, error) {
	return headlessAlive(h.procs(), meta)
}

// headlessAlive reports whether meta's claude still runs: the pid exists,
// started when meta says, and still carries the session id. A pid the OS
// reused for another process fails the start-time or cmdline check.
func headlessAlive(procs ProcessTable, meta Meta) (bool, error) {
	return pidAlive(procs, meta.PID, meta.PIDStart, meta.SessionID)
}

// ptyAlive reports whether both the holder and its claude still run, each
// checked like headlessAlive. Losing either one leaves no usable agent.
func ptyAlive(procs ProcessTable, meta Meta) (bool, error) {
	live, err := pidAlive(procs, meta.HolderPID, meta.HolderPIDStart, meta.SessionID)
	if err != nil || !live {
		return false, err
	}
	return headlessAlive(procs, meta)
}

func pidAlive(procs ProcessTable, pid int, start, sessionID string) (bool, error) {
	proc, ok, err := procs.Lookup(pid)
	if err != nil || !ok {
		return false, err
	}
	return start != "" && proc.Start == start &&
		strings.Contains(proc.Cmdline, sessionID), nil
}

// watchExit closes ss.exited once an adopted claude is gone. It isn't the
// server's child, so there is no Wait to block on.
func (h *Headless) watchExit(ss *headlessSession, meta Meta) {
	ticker := time.NewTicker(h.pollInterval())
	defer ticker.Stop()
	for range ticker.C {
		// A failed lookup is retried rather than taken as an exit.
		if live, err := h.alive(meta); err == nil && !live {
			close(ss.exited)
			return
		}
	}
}

// replay folds the complete lines of out.jsonl from offset into t and
// returns the offset after the last one. A partial last line is a write
// still in flight (or cut short by a crash) and is left for the tailer.
func replay(path string, offset int64, t *tracker) (int64, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return offset, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if err == io.EOF {
			return offset, nil
		}
		if err != nil {
			return 0, err
		}
		offset += int64(len(line))
		if e, ok := parseEvent(line); ok {
			t.apply(e)
		}
	}
}

func readMeta(dir string) (Meta, error) {
	data, err := os.ReadFile(filepath.Join(dir, MetaFile))
	if errors.Is(err, os.ErrNotExist) {
		return Meta{}, fmt.Errorf("%w: %s", agentrunner.ErrNotFound, filepath.Base(dir))
	}
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return Meta{}, fmt.Errorf("%s: %w", filepath.Join(dir, MetaFile), err)
	}
	return m, nil
}
