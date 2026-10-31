package servertest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

// SeedEarlierClaudeRun records an earlier Claude run of epic/id in the run log
// (an iteration-started event with a session). With transcript set it also
// writes the session's transcript under the harness's HOME, as a run that got
// as far as its first prompt does; without it the run died before that.
func SeedEarlierClaudeRun(t *testing.T, h *Harness, store, project, epic, id, session string, withTranscript bool) {
	t.Helper()
	const cwd = "/work/earlier-run"
	err := ralphloop.AppendEvent(filepath.Join(store, project), epic, ralphloop.Event{
		Time: time.Now(), Type: string(events.IterationStarted), Ticket: id,
		Agent: ralphloop.AgentClaude, AgentSession: session, Cwd: cwd,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !withTranscript {
		return
	}
	path := transcript.PathIn(h.Env("HOME"), cwd, session)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
