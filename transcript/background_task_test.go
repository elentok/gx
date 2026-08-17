package transcript

import (
	"path/filepath"
	"testing"
	"time"
)

func startMarkerLine(taskID, toolUseID, timestamp string) string {
	return `{"isSidechain":false,"timestamp":"` + timestamp + `","message":{"content":[{"tool_use_id":"` + toolUseID + `","type":"tool_result","content":"...","is_error":false}]},"toolUseResult":{"stdout":"","backgroundTaskId":"` + taskID + `"}}`
}

func sidechainStartMarkerLine(taskID, toolUseID, timestamp string) string {
	return `{"isSidechain":true,"timestamp":"` + timestamp + `","message":{"content":[{"tool_use_id":"` + toolUseID + `","type":"tool_result","content":"...","is_error":false}]},"toolUseResult":{"stdout":"","backgroundTaskId":"` + taskID + `"}}`
}

func notificationLine(taskID, toolUseID, status, timestamp string) string {
	content := "<task-notification>\\n<task-id>" + taskID + "</task-id>\\n<tool-use-id>" + toolUseID + "</tool-use-id>\\n<status>" + status + "</status>\\n</task-notification>"
	return `{"isSidechain":false,"timestamp":"` + timestamp + `","origin":{"kind":"task-notification"},"message":{"content":"` + content + `"}}`
}

func sidechainNotificationLine(taskID, toolUseID, status, timestamp string) string {
	content := "<task-notification>\\n<task-id>" + taskID + "</task-id>\\n<tool-use-id>" + toolUseID + "</tool-use-id>\\n<status>" + status + "</status>\\n</task-notification>"
	return `{"isSidechain":true,"timestamp":"` + timestamp + `","origin":{"kind":"task-notification"},"message":{"content":"` + content + `"}}`
}

// taskOutputLine mirrors the tool_result a blocking TaskOutput call gets:
// this never carries origin.kind == "task-notification" (no passive
// notification is ever queued once the tool retrieves the result directly).
func taskOutputLine(taskID, timestamp string) string {
	content := "<retrieval_status>success</retrieval_status>\\n\\n<task_id>" + taskID + "</task_id>\\n\\n<status>completed</status>"
	return `{"isSidechain":false,"timestamp":"` + timestamp + `","message":{"content":[{"tool_use_id":"tool-out","type":"tool_result","content":"` + content + `"}]},"toolUseResult":{"retrieval_status":"success","task":{"task_id":"` + taskID + `","status":"completed"}}}`
}

// taskStopLine mirrors the tool_result a TaskStop call gets: a flat
// toolUseResult.task_id, no retrieval_status/nested task and no
// task-notification entry of its own either.
func taskStopLine(taskID, timestamp string) string {
	content := `{\"message\":\"Successfully stopped task: ` + taskID + `\",\"task_id\":\"` + taskID + `\"}`
	return `{"isSidechain":false,"timestamp":"` + timestamp + `","message":{"content":[{"tool_use_id":"tool-stop","type":"tool_result","content":"` + content + `"}]},"toolUseResult":{"message":"Successfully stopped task: ` + taskID + `","task_id":"` + taskID + `"}}`
}

// queueOperationAttachmentLine mirrors the queue-operation/attachment shape
// found in the incident that produced this ticket: neither a
// task-notification, nor a TaskOutput/TaskStop tool_result — the task id
// only ever shows up embedded in an attachment payload.
func queueOperationAttachmentLine(taskID, timestamp string) string {
	return `{"isSidechain":false,"timestamp":"` + timestamp + `","type":"queue-operation","operation":"attachment","attachment":{"taskId":"` + taskID + `","kind":"background-task-result"}}`
}

const capDuration = 2 * time.Hour

var readAt = mustParseTime("2026-08-12T18:00:00.000000000Z")

func mustParseTime(s string) time.Time {
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return ts
}

func TestReadBackgroundTasks_NoBackgroundTaskAtAll(t *testing.T) {
	path := writeTranscript(t, `{"type":"assistant","message":{}}`)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if reading.FileStatus != "" {
		t.Errorf("FileStatus = %q, want empty", reading.FileStatus)
	}
	if len(reading.Markers) != 0 {
		t.Errorf("Markers = %+v, want none", reading.Markers)
	}
}

func TestReadBackgroundTasks_Resolved_NonCompletedStatusStillResolves(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T15:00:00.000000000Z"),
		notificationLine("task-1", "tool-1", "failed", "2026-08-12T15:05:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskResolved {
		t.Errorf("Markers = %+v, want one resolved marker (a failed/killed notification still resolves)", reading.Markers)
	}
}

func TestReadBackgroundTasks_ResolvedViaBlockingTaskOutputRetrieval(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T15:00:00.000000000Z"),
		taskOutputLine("task-1", "2026-08-12T15:05:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskResolved {
		t.Errorf("Markers = %+v, want one resolved marker: a blocking TaskOutput retrieval resolves the marker even with no task-notification entry", reading.Markers)
	}
}

func TestReadBackgroundTasks_ResolvedViaTaskStop(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T15:00:00.000000000Z"),
		taskStopLine("task-1", "2026-08-12T15:05:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskResolved {
		t.Errorf("Markers = %+v, want one resolved marker: killing the task via TaskStop resolves the marker even with no task-notification entry", reading.Markers)
	}
}

func TestReadBackgroundTasks_OutstandingFresh(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskOutstandingFresh {
		t.Errorf("Markers = %+v, want one outstanding-fresh marker", reading.Markers)
	}
}

func TestReadBackgroundTasks_OutstandingAgedOut(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T10:00:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskOutstandingAgedOut {
		t.Errorf("Markers = %+v, want one outstanding-aged-out marker", reading.Markers)
	}
}

func TestReadBackgroundTasks_SidechainMarkerNeverReported(t *testing.T) {
	path := writeTranscript(t,
		sidechainStartMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 0 {
		t.Errorf("Markers = %+v, want none — a sidechain marker must never be reported", reading.Markers)
	}
}

func TestReadBackgroundTasks_OrphanNotificationIgnoredAndDoesNotAffectOtherMarkers(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z"),
		notificationLine("task-orphan", "tool-orphan", "completed", "2026-08-12T17:05:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].TaskID != "task-1" || reading.Markers[0].Status != BackgroundTaskOutstandingFresh {
		t.Errorf("Markers = %+v, want task-1 still outstanding-fresh, unaffected by the orphan notification", reading.Markers)
	}
}

func TestReadBackgroundTasks_MatchesOnTaskIDEvenWithoutToolUseIDCorroboration(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T15:00:00.000000000Z"),
		`{"isSidechain":false,"timestamp":"2026-08-12T15:05:00.000000000Z","origin":{"kind":"task-notification"},"message":{"content":"<task-notification>\n<task-id>task-1</task-id>\n<status>completed</status>\n</task-notification>"}}`,
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskResolved {
		t.Errorf("Markers = %+v, want task-1 resolved by task id alone, no tool-use-id required", reading.Markers)
	}
}

func TestReadBackgroundTasks_ResolvedViaQueueOperationAttachment(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T15:00:00.000000000Z"),
		queueOperationAttachmentLine("task-1", "2026-08-12T15:05:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskResolved {
		t.Errorf("Markers = %+v, want one resolved marker: an unrecognized queue-operation/attachment shape still resolves by id-substring match", reading.Markers)
	}
}

func TestReadBackgroundTasks_SidechainNotificationNeverResolvesParentMarker(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z"),
		sidechainNotificationLine("task-1", "tool-1", "completed", "2026-08-12T17:05:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskOutstandingFresh {
		t.Errorf("Markers = %+v, want task-1 still outstanding-fresh — a sidechain-owned notification must never resolve the parent iteration's marker, even with a matching task id", reading.Markers)
	}
}

func TestReadBackgroundTasks_UnreadableFile(t *testing.T) {
	path := writeTranscript(t, "not json at all", "still not json")

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if reading.FileStatus != BackgroundTaskUnreadable {
		t.Errorf("FileStatus = %q, want %q", reading.FileStatus, BackgroundTaskUnreadable)
	}
	if len(reading.Markers) != 0 {
		t.Errorf("Markers = %+v, want none for an unreadable file", reading.Markers)
	}
}

func TestReadBackgroundTasks_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.jsonl")

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if reading.FileStatus != BackgroundTaskUnsupported {
		t.Errorf("FileStatus = %q, want %q", reading.FileStatus, BackgroundTaskUnsupported)
	}
	if len(reading.Markers) != 0 {
		t.Errorf("Markers = %+v, want none for a missing file", reading.Markers)
	}
}
