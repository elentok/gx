package claudeui

import (
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/transcript"
)

// BackgroundTasks returns a Runner.BackgroundTasks probe that reads the
// Claude transcript under home. It finds the transcript by session id rather
// than cwd, so it also works for sessions recovered by Find or List. A Codex
// session has no such transcript, so it never gates. A read error counts as
// no task running, as in ralph-loop's own gate.
//
// The probe runs every few seconds per held agent, and FindByID globs every
// project dir, so the found path is cached per session id. A failed read drops
// the entry and searches again.
func BackgroundTasks(home string, now func() time.Time) func(agentrunner.Session) bool {
	var mu sync.Mutex
	known := map[string]string{}
	return func(s agentrunner.Session) bool {
		if s.SessionID == "" {
			return false
		}
		mu.Lock()
		path, cached := known[s.SessionID]
		mu.Unlock()
		var reading transcript.BackgroundTaskReading
		err := errors.New("not cached")
		if cached {
			reading, err = transcript.ReadBackgroundTasks(path, transcript.BackgroundTaskAgedOutCap, now())
		}
		// A missing file is not an error to ReadBackgroundTasks; it reports Unsupported.
		if err != nil || reading.FileStatus == transcript.BackgroundTaskUnsupported {
			paths, ferr := transcript.FindByID(home, s.SessionID)
			if ferr != nil || len(paths) != 1 {
				return false
			}
			path = paths[0]
			reading, err = transcript.ReadBackgroundTasks(path, transcript.BackgroundTaskAgedOutCap, now())
			if err != nil {
				return false
			}
			mu.Lock()
			known[s.SessionID] = path
			mu.Unlock()
		}
		return slices.ContainsFunc(reading.Markers, func(m transcript.BackgroundTaskMarker) bool {
			return m.Status == transcript.BackgroundTaskOutstandingFresh
		})
	}
}
