package claudeui

import (
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/backgroundtask"
)

// BackgroundTasks returns a Runner.BackgroundTasks probe that reads the
// Claude transcript under home through the shared backgroundtask reading. It
// finds the transcript by session id rather than cwd, so it also works for
// sessions recovered by Find or List. A Codex session has no such transcript,
// so it never gates. A read error counts as no task running.
func BackgroundTasks(home string, now func() time.Time) func(agentrunner.Session) bool {
	rd := &backgroundtask.Reader{Home: home, Now: now}
	return func(s agentrunner.Session) bool {
		return rd.Read(s.SessionID, backgroundtask.PidUnknown).Live
	}
}
