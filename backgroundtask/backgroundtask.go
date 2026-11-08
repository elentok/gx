// Package backgroundtask is the one reading of "a background task is live"
// that the finish wait, the stall nudge, the zero-commit hold and the TUI
// +bg label all share.
package backgroundtask

import (
	"errors"
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

// Reader reads a session's transcript by session id. The probe runs every few
// seconds per held agent, and FindByID globs every project dir, so the found
// path is cached per session id.
type Reader struct {
	Home string
	Now  func() time.Time

	mu    sync.Mutex
	known map[string]string
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
		tr, err := transcript.ReadBackgroundTasks(path, transcript.BackgroundTaskAgedOutCap, now())
		if err == nil && tr.FileStatus != transcript.BackgroundTaskUnsupported {
			return tr, nil
		}
	}
	paths, err := transcript.FindByID(rd.Home, sessionID)
	if err != nil {
		return transcript.BackgroundTaskReading{}, err
	}
	if len(paths) != 1 {
		return transcript.BackgroundTaskReading{}, errors.New("transcript not found or ambiguous")
	}
	tr, err := transcript.ReadBackgroundTasks(paths[0], transcript.BackgroundTaskAgedOutCap, now())
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
