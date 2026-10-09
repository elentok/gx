package herdrrunner

import (
	"slices"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/transcript"
)

// ClaudeBackgroundTasks returns a Runner.BackgroundTasks probe that reads the
// Claude transcript under home. It finds the transcript by session id rather
// than cwd, so it also works for sessions recovered by Find or List. A Codex
// session has no such transcript, so it never gates. A read error counts as
// no task running, as in ralph-loop's own gate.
func ClaudeBackgroundTasks(home string, now func() time.Time) func(agentrunner.Session) bool {
	return func(s agentrunner.Session) bool {
		if s.SessionID == "" {
			return false
		}
		paths, err := transcript.FindByID(home, s.SessionID)
		if err != nil || len(paths) != 1 {
			return false
		}
		reading, err := transcript.ReadBackgroundTasks(paths[0], transcript.BackgroundTaskAgedOutCap, now())
		if err != nil {
			return false
		}
		return slices.ContainsFunc(reading.Markers, func(m transcript.BackgroundTaskMarker) bool {
			return m.Status == transcript.BackgroundTaskOutstandingFresh
		})
	}
}
