package backgroundtask

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/transcript"
)

// Facts is what the gate card shows about the live task.
type Facts struct {
	// TaskID and Kind come from the task's transcript marker.
	TaskID    string
	Kind      string
	Label     string
	StartedAt time.Time
	// Process is the match of the task to its process; zero when the output
	// file was not found.
	Process ProcessMatch
}

// DefaultOutputBase is the directory claude keeps its task output files under.
func DefaultOutputBase() string {
	return filepath.Join("/tmp", fmt.Sprintf("claude-%d", os.Getuid()))
}

// OutputPath finds the task's .output file under base, or "" when there is not
// exactly one.
func OutputPath(base, sessionID, taskID string) string {
	paths, err := filepath.Glob(filepath.Join(base, "*", sessionID, "tasks", taskID+".output"))
	if err != nil || len(paths) != 1 {
		return ""
	}
	return paths[0]
}

// Facts reads the newest outstanding task of sessionID and matches it to its
// process. ok is false when the session has no outstanding task.
func (rd *Reader) Facts(sessionID, outputBase string, finder ProcessFinder) (f Facts, ok bool) {
	var newest transcript.BackgroundTaskMarker
	for _, m := range rd.Read(sessionID, PidUnknown).Tasks {
		if m.Status == transcript.BackgroundTaskOutstandingFresh && (!ok || m.StartedAt.After(newest.StartedAt)) {
			newest, ok = m, true
		}
	}
	if !ok {
		return Facts{}, false
	}
	f = Facts{TaskID: newest.TaskID, Kind: newest.Kind, Label: newest.Label, StartedAt: newest.StartedAt}
	// Root pid 0 spans the whole table: the output-file match is unique per task.
	f.Process, _ = finder.Match(0, OutputPath(outputBase, sessionID, newest.TaskID), "")
	return f, true
}
