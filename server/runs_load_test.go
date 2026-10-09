package server

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elentok/gx/agentrunner"
)

func TestOpenRuns_OldFileLoadsAsHerdrSession(t *testing.T) {
	dir := t.TempDir()
	old := `[{"address":"proj:epic-a/01","agent":"claude","pane":"p1","tab":"t1","root":"proj:epic-a"}]`
	if err := os.WriteFile(filepath.Join(dir, runsFileName), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	_, saved, err := openRuns(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Run{
		Address: "proj:epic-a/01", Agent: "claude", Runner: runnerHerdr,
		Session: agentrunner.Session{Label: iterationLabel("proj:epic-a/01"), ID: "p1"},
	}
	if len(saved) != 1 || !reflect.DeepEqual(saved[0].Run, want) || saved[0].Root != "proj:epic-a" {
		t.Fatalf("saved = %+v, want run %+v", saved, want)
	}
}

func TestOpenRuns_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	r, _, err := openRuns(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := trackedRun{
		Run: Run{
			Address: "proj:epic-a/01", Agent: "claude", Runner: "headless",
			Session: agentrunner.Session{Label: "epic-a-01", ID: "epic-a-01", SessionID: "sess-1"},
		},
		Root: "proj:epic-a",
	}
	r.put(run)
	_, saved, err := openRuns(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || !reflect.DeepEqual(saved[0], run) {
		t.Fatalf("saved = %+v, want %+v", saved, run)
	}
}
