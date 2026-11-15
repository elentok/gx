package recovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/runlog"
)

// A run log written before finish-blocked, skills-refreshed and launch-retried
// existed still reads, and its counts for the new events are zero.
func TestCountGxEvents_RunLogWithoutNewEventsLoadsUnchanged(t *testing.T) {
	scratch := t.TempDir()
	path := runlog.RunLogPath(scratch, "epic")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const old = `{"time":"2026-01-02T03:04:05Z","type":"iteration-started","ticket":"01"}
{"time":"2026-01-02T03:05:05Z","type":"prompt-nudged","ticket":"01"}
{"time":"2026-01-02T03:06:05Z","type":"launch-failed","ticket":"01","reason":"boom"}
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	evs, ok, err := runlog.ReadEvents(scratch, "epic")
	if err != nil || !ok || len(evs) != 3 {
		t.Fatalf("ReadEvents = %d events, ok=%v, err=%v", len(evs), ok, err)
	}
	c := countGxEvents(evs, "01", time.Time{})
	if c.resends != 1 || c.launchFailures != 1 {
		t.Errorf("counts = %+v, want one resend and one launch failure", c)
	}
	if c.finishBlocks != 0 || c.refreshes != 0 || c.retries != 0 {
		t.Errorf("new-event counts = %+v, want zero", c)
	}
	if got := gxWentWrong(c); got != "1 prompt resend, 1 launch failure" {
		t.Errorf("went wrong = %q", got)
	}
}
