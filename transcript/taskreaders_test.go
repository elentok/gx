package transcript

import "testing"

// The task list reads ReadTaskRecords and the ralphloop finish gate reads
// ReadBackgroundTasks. Both must call a task running or finished alike.
func TestTaskReaders_AgreeOnRunning(t *testing.T) {
	const start = "2026-08-12T17:00:00.000000000Z"
	const later = "2026-08-12T17:05:00.000000000Z"
	for _, tc := range []struct {
		name  string
		id    string
		lines []string
		want  bool // running
	}{
		{"shell outstanding", "task-1", []string{startMarkerLine("task-1", "u1", start)}, true},
		{"shell notified", "task-1", []string{startMarkerLine("task-1", "u1", start), notificationLine("task-1", "u1", "completed", later)}, false},
		{"shell queue-operation notice", "task-1", []string{startMarkerLine("task-1", "u1", start), enqueueLine("task-1", "completed", "done", later)}, false},
		{"shell TaskStop", "task-1", []string{startMarkerLine("task-1", "u1", start), taskStopLine("task-1", later)}, false},
		{"TaskStop of another task", "task-1", []string{startMarkerLine("task-1", "u1", start), taskStopLine("poll-1", later)}, true},
		{"monitor outstanding", "mon-1", []string{monitorStartLine("mon-1", start)}, true},
		{"monitor notified", "mon-1", []string{monitorStartLine("mon-1", start), notificationLine("mon-1", "u1", "completed", later)}, false},
		{"async agent outstanding", "agent-1", []string{asyncAgentLaunchLine("agent-1", start)}, true},
		{"async agent interim notice", "agent-1", []string{asyncAgentLaunchLine("agent-1", start), interimAgentNotificationLine("agent-1", later)}, true},
		{"async agent final notice", "agent-1", []string{asyncAgentLaunchLine("agent-1", start), notificationLine("agent-1", "u1", "completed", later)}, false},
	} {
		path := writeTranscript(t, tc.lines...)

		reading, err := ReadBackgroundTasks(path, capDuration, readAt)
		if err != nil {
			t.Fatal(err)
		}
		viewRunning := len(reading.Markers) == 1 && reading.Markers[0].TaskID == tc.id && reading.Markers[0].Status != BackgroundTaskResolved

		recs, _, err := ReadTaskRecords(path)
		if err != nil {
			t.Fatal(err)
		}
		recRunning := len(recs) == 1 && recs[0].TaskID == tc.id && recs[0].End == nil

		if viewRunning != tc.want || recRunning != tc.want {
			t.Errorf("%s: ReadBackgroundTasks running = %v, ReadTaskRecords running = %v, want %v", tc.name, viewRunning, recRunning, tc.want)
		}
	}
}
