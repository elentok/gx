package backgroundtask_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/backgroundtask"
	"github.com/elentok/gx/syshealth"
)

// A live task reads its label and start from the transcript and its output
// facts from the task's output file under the output base.
func TestReader_FactsOfLiveTask(t *testing.T) {
	home, base := t.TempDir(), t.TempDir()
	const session, taskID = "sess-1", "bg1"
	started := time.Now().Add(-90 * time.Second).UTC()
	projects := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	launch := `{"isSidechain":false,"type":"assistant","timestamp":"` + started.Format(time.RFC3339Nano) + `","message":{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"tu1","name":"Bash","input":{"description":"run the soak","command":"make soak","run_in_background":true}}]}}`
	start := `{"isSidechain":false,"type":"user","timestamp":"` + started.Add(time.Second).Format(time.RFC3339Nano) + `","message":{"content":[{"tool_use_id":"tu1","type":"tool_result","content":"...","is_error":false}]},"toolUseResult":{"stdout":"","backgroundTaskId":"` + taskID + `"}}`
	if err := os.WriteFile(filepath.Join(projects, session+".jsonl"), []byte(launch+"\n"+start+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks := filepath.Join(base, "x", session, "tasks")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tasks, taskID+".output")
	startSleeper(t, out)
	if err := os.WriteFile(out, []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rd := &backgroundtask.Reader{Home: home}
	f, ok := rd.Facts(session, base, backgroundtask.ProcessFinder{Procs: syshealth.DefaultProcess()})
	if !ok {
		t.Fatal("no live task found")
	}
	if f.Label != "run the soak" || !f.Process.Alive || f.Process.LastOutput.IsZero() {
		t.Fatalf("facts = %+v", f)
	}
	if want := []string{"b", "c", "d"}; !slices.Equal(f.Process.LastLines, want) {
		t.Fatalf("lines = %v, want %v", f.Process.LastLines, want)
	}
	if _, ok := rd.Facts("other", base, backgroundtask.ProcessFinder{Procs: syshealth.DefaultProcess()}); ok {
		t.Fatal("a session without a task reported facts")
	}
}
