// Package backgroundtask is the one reading of "a background task is live"
// that the finish wait, the stall nudge, the zero-commit hold and the TUI
// +bg label all share.
package backgroundtask

import (
	"errors"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/transcript"
)

// Source is what a Reading's Live rests on.
type Source string

const (
	SourceNone       Source = "none"
	SourceTranscript Source = "transcript"
	SourcePidFile    Source = "pidfile"
	SourceBoth       Source = "both"
)

// Pid is what claude's pid status file says about a background task.
type Pid int

const (
	// PidUnknown means the caller has no pid file; the transcript decides.
	PidUnknown Pid = iota
	// PidShell is the pid file's `shell` state: idle with a task running.
	PidShell
	// PidNoShell is any other pid-file state.
	PidNoShell
)

// Reading is the shared answer.
type Reading struct {
	// Live is true while the agent still waits on a task.
	Live bool
	// Tasks are all of the transcript's markers, whatever their status, so a
	// caller can log each one's release.
	Tasks []transcript.BackgroundTaskMarker
	// AwaitingTurnEnd is a notification-woken turn that has not ended.
	AwaitingTurnEnd bool
	Source          Source
	// Disagree is a pid-file `shell` that a clean transcript cleared as stale.
	Disagree bool
}

// Resolve applies the one precedence rule. The pid file's status wins: a
// `shell` stands as live unless a transcript that read cleanly shows no
// outstanding task, which clears it as stale. Any other pid state defers to
// the transcript. A read error or a missing transcript counts as not live,
// and cannot clear a `shell`.
func Resolve(pid Pid, tr transcript.BackgroundTaskReading, readErr error) Reading {
	r := Reading{Source: SourceNone}
	readable := readErr == nil && tr.FileStatus == ""
	if readable {
		r.Tasks, r.AwaitingTurnEnd = tr.Markers, tr.AwaitingTurnEnd
	}
	transcriptLive := readable && (r.AwaitingTurnEnd || slices.ContainsFunc(tr.Markers, func(m transcript.BackgroundTaskMarker) bool {
		return m.Status == transcript.BackgroundTaskOutstandingFresh
	}))
	switch {
	case pid == PidShell && transcriptLive:
		r.Live, r.Source = true, SourceBoth
	case pid == PidShell && readable:
		r.Disagree = true
	case pid == PidShell:
		r.Live, r.Source = true, SourcePidFile
	case transcriptLive:
		r.Live, r.Source = true, SourceTranscript
	}
	return r
}

// globMissEvery spaces the glob for a session whose transcript is missing.
const globMissEvery = 5 * time.Second

var errNoTranscript = errors.New("transcript not found or ambiguous")

// Reader reads a session's transcript by session id. The probe runs every few
// seconds per held agent, and FindByID globs every project dir, so the found
// path is cached per session id.
type Reader struct {
	Home string
	Now  func() time.Time
	// Glob is a test seam; nil means transcript.FindByID.
	Glob func(home, sessionID string) ([]string, error)

	mu    sync.Mutex
	known map[string]string
	// missed is when each session's last glob found no single transcript, so a
	// held agent without one does not glob every project dir on each poll.
	missed map[string]time.Time
	// last is the newest parse per session id. An idle poll then costs one
	// stat instead of a whole-transcript parse.
	last map[string]cachedReading
}

type cachedReading struct {
	path    string
	size    int64
	modTime time.Time
	reading transcript.BackgroundTaskReading
}

// parse reads the transcript at path, serving the cached parse while the file's
// size and mtime are unchanged and no marker has aged out since.
func (rd *Reader) parse(sessionID, path string, now time.Time) (transcript.BackgroundTaskReading, error) {
	fi, statErr := os.Stat(path)
	if statErr == nil {
		rd.mu.Lock()
		c, ok := rd.last[sessionID]
		rd.mu.Unlock()
		if ok && c.path == path && c.size == fi.Size() && c.modTime.Equal(fi.ModTime()) &&
			(c.reading.ValidUntil.IsZero() || !now.After(c.reading.ValidUntil)) {
			return c.reading, nil
		}
	}
	tr, err := transcript.ReadBackgroundTasks(path, transcript.BackgroundTaskAgedOutCap, now)
	if err == nil && statErr == nil && tr.FileStatus == "" {
		rd.mu.Lock()
		if rd.last == nil {
			rd.last = map[string]cachedReading{}
		}
		rd.last[sessionID] = cachedReading{path: path, size: fi.Size(), modTime: fi.ModTime(), reading: tr}
		rd.mu.Unlock()
	}
	return tr, err
}

// Read returns the reading for sessionID. A Codex session has no transcript,
// so it is never live.
func (rd *Reader) Read(sessionID string, pid Pid) Reading {
	if sessionID == "" {
		return Resolve(pid, transcript.BackgroundTaskReading{FileStatus: transcript.BackgroundTaskUnsupported}, nil)
	}
	tr, err := rd.read(sessionID)
	return Resolve(pid, tr, err)
}

func (rd *Reader) read(sessionID string) (transcript.BackgroundTaskReading, error) {
	now := rd.Now
	if now == nil {
		now = time.Now
	}
	rd.mu.Lock()
	path, cached := rd.known[sessionID]
	rd.mu.Unlock()
	if cached {
		tr, err := rd.parse(sessionID, path, now())
		if err == nil && tr.FileStatus != transcript.BackgroundTaskUnsupported {
			return tr, nil
		}
	}
	rd.mu.Lock()
	missedAt, missed := rd.missed[sessionID]
	rd.mu.Unlock()
	if missed && now().Sub(missedAt) < globMissEvery {
		return transcript.BackgroundTaskReading{}, errNoTranscript
	}
	find := rd.Glob
	if find == nil {
		find = transcript.FindByID
	}
	paths, err := find(rd.Home, sessionID)
	if err != nil {
		return transcript.BackgroundTaskReading{}, err
	}
	if len(paths) != 1 {
		rd.mu.Lock()
		if rd.missed == nil {
			rd.missed = map[string]time.Time{}
		}
		rd.missed[sessionID] = now()
		rd.mu.Unlock()
		return transcript.BackgroundTaskReading{}, errNoTranscript
	}
	tr, err := rd.parse(sessionID, paths[0], now())
	if err != nil {
		return tr, err
	}
	rd.mu.Lock()
	if rd.known == nil {
		rd.known = map[string]string{}
	}
	rd.known[sessionID] = paths[0]
	rd.mu.Unlock()
	return tr, nil
}
