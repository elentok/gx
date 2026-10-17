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

func TestReadBackgroundTasks_PrefixCollidingLongerIDDoesNotResolveShorterMarker(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z"),
		notificationLine("task-10", "tool-10", "completed", "2026-08-12T17:05:00.000000000Z"),
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].TaskID != "task-1" || reading.Markers[0].Status != BackgroundTaskOutstandingFresh {
		t.Errorf("Markers = %+v, want task-1 still outstanding-fresh — a later line mentioning only task-10 must not resolve task-1's marker", reading.Markers)
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

// An agent that watches its own background task (a Monitor on the task's output
// path) names the id in its tool call; that is not the task finishing.
func TestReadBackgroundTasks_AssistantMentioningTheTaskIDDoesNotResolveIt(t *testing.T) {
	mention := `{"isSidechain":false,"type":"assistant","timestamp":"2026-08-12T17:00:05.000000000Z","message":{"content":[{"type":"tool_use","name":"Monitor","input":{"command":"until rg -q ok /tmp/tasks/task-1.output; do sleep 5; done"}}]}}`
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z"),
		mention,
	)

	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatalf("ReadBackgroundTasks() error = %v", err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskOutstandingFresh {
		t.Errorf("Markers = %+v, want one outstanding-fresh marker", reading.Markers)
	}
}

// A Monitor start (taskId + timeoutMs, no backgroundTaskId) holds the finish
// gate like a backgrounded shell command until its notification arrives.
func monitorStartLine(taskID, timestamp string) string {
	return `{"isSidechain":false,"type":"user","timestamp":"` + timestamp + `","message":{"content":[{"type":"tool_result","content":"Monitor started"}]},"toolUseResult":{"taskId":"` + taskID + `","timeoutMs":400000,"persistent":false}}`
}

func TestReadBackgroundTasks_MonitorStartHoldsUntilItsNotification(t *testing.T) {
	soon := mustParseTime("2026-08-12T17:02:00.000000000Z") // inside the Monitor's own timeout
	path := writeTranscript(t, monitorStartLine("mon-1", "2026-08-12T17:00:00.000000000Z"))
	reading, err := ReadBackgroundTasks(path, capDuration, soon)
	if err != nil {
		t.Fatal(err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].TaskID != "mon-1" || reading.Markers[0].Status != BackgroundTaskOutstandingFresh {
		t.Fatalf("Markers = %+v, want mon-1 outstanding-fresh", reading.Markers)
	}

	path = writeTranscript(t,
		monitorStartLine("mon-1", "2026-08-12T17:00:00.000000000Z"),
		notificationLine("mon-1", "tool-1", "completed", "2026-08-12T17:05:00.000000000Z"),
	)
	reading, err = ReadBackgroundTasks(path, capDuration, soon)
	if err != nil {
		t.Fatal(err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskResolved {
		t.Errorf("Markers = %+v, want mon-1 resolved by its notification", reading.Markers)
	}
}

// A taskId without timeoutMs is some other tool's result, not a Monitor.
func TestReadBackgroundTasks_TaskIDWithoutTimeoutIsNotAMarker(t *testing.T) {
	line := `{"isSidechain":false,"type":"user","timestamp":"2026-08-12T17:00:00.000000000Z","toolUseResult":{"taskId":"todo-1"}}`
	reading, err := ReadBackgroundTasks(writeTranscript(t, line), capDuration, readAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(reading.Markers) != 0 {
		t.Errorf("Markers = %+v, want none", reading.Markers)
	}
}

func asyncAgentLaunchLine(agentID, timestamp string) string {
	return `{"isSidechain":false,"type":"user","timestamp":"` + timestamp + `","message":{"content":[{"type":"tool_result","content":"Async agent launched"}]},"toolUseResult":{"isAsync":true,"status":"async_launched","agentId":"` + agentID + `","description":"Migrate tests"}}`
}

// interimAgentNotificationLine mirrors the notice Claude Code sends when a
// subagent ends its turn waiting on background work of its own.
func interimAgentNotificationLine(agentID, timestamp string) string {
	content := "<task-notification>\\n<task-id>" + agentID + "</task-id>\\n<status>completed</status>\\n<note>This agent stopped with background work of its own still running. It may resume on its own when that work completes or reports, and the same task-id notifies again if it does; the result below may be interim.</note>\\n</task-notification>"
	return `{"isSidechain":false,"type":"user","timestamp":"` + timestamp + `","origin":{"kind":"task-notification"},"message":{"content":"` + content + `"}}`
}

func TestReadBackgroundTasks_AsyncAgentHoldsThroughInterimNotification(t *testing.T) {
	launch := asyncAgentLaunchLine("agent-1", "2026-08-12T17:00:00.000000000Z")
	interim := interimAgentNotificationLine("agent-1", "2026-08-12T17:20:00.000000000Z")
	final := notificationLine("agent-1", "tool-1", "completed", "2026-08-12T17:40:00.000000000Z")
	for _, tc := range []struct {
		name  string
		lines []string
		want  BackgroundTaskStatus
	}{
		{"launched", []string{launch}, BackgroundTaskOutstandingFresh},
		{"interim only", []string{launch, interim}, BackgroundTaskOutstandingFresh},
		{"final after interim", []string{launch, interim, final}, BackgroundTaskResolved},
	} {
		reading, err := ReadBackgroundTasks(writeTranscript(t, tc.lines...), capDuration, readAt)
		if err != nil {
			t.Fatal(err)
		}
		if len(reading.Markers) != 1 || reading.Markers[0].TaskID != "agent-1" || reading.Markers[0].Status != tc.want {
			t.Errorf("%s: Markers = %+v, want agent-1 %s", tc.name, reading.Markers, tc.want)
		}
	}
}

func TestReadBackgroundTasks_AsyncAgentAgesOutAtTheCap(t *testing.T) {
	path := writeTranscript(t,
		asyncAgentLaunchLine("agent-1", "2026-08-12T15:00:00.000000000Z"),
		interimAgentNotificationLine("agent-1", "2026-08-12T15:20:00.000000000Z"),
	)
	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskOutstandingAgedOut {
		t.Errorf("Markers = %+v, want agent-1 outstanding-aged-out", reading.Markers)
	}
}

func assistantLine(stopReason, timestamp string) string {
	return `{"isSidechain":false,"type":"assistant","timestamp":"` + timestamp + `","message":{"stop_reason":"` + stopReason + `","content":[{"type":"text","text":"..."}]}}`
}

// A notification wakes an agent that ended its turn waiting on it, so the
// iteration isn't over until the agent ends the turn that notification began.
func TestReadBackgroundTasks_AwaitsTurnEndAfterResolution(t *testing.T) {
	start := startMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z")
	waiting := assistantLine("end_turn", "2026-08-12T17:00:05.000000000Z")
	notice := notificationLine("task-1", "tool-1", "completed", "2026-08-12T17:10:00.000000000Z")
	toolCall := assistantLine("tool_use", "2026-08-12T17:10:03.000000000Z")
	done := assistantLine("end_turn", "2026-08-12T17:12:00.000000000Z")
	for _, tc := range []struct {
		name  string
		lines []string
		now   string
		want  bool
	}{
		{"outstanding", []string{start, waiting}, "2026-08-12T18:00:00.000000000Z", false},
		{"notified, no reply yet", []string{start, waiting, notice}, "2026-08-12T18:00:00.000000000Z", true},
		{"notified, mid-turn", []string{start, waiting, notice, toolCall}, "2026-08-12T18:00:00.000000000Z", true},
		{"notified, turn ended", []string{start, waiting, notice, toolCall, done}, "2026-08-12T18:00:00.000000000Z", false},
		{"turn never ended, past the cap", []string{start, waiting, notice, toolCall}, "2026-08-12T19:11:00.000000000Z", false},
	} {
		reading, err := ReadBackgroundTasks(writeTranscript(t, tc.lines...), capDuration, mustParseTime(tc.now))
		if err != nil {
			t.Fatal(err)
		}
		if reading.AwaitingTurnEnd != tc.want {
			t.Errorf("%s: AwaitingTurnEnd = %v, want %v", tc.name, reading.AwaitingTurnEnd, tc.want)
		}
	}
}

// readOutputLine mirrors the tool_result of the agent's own Read of a task's
// .output file: it names the task id in a path, but the CLI isn't reporting on
// the task.
func readOutputLine(taskID, timestamp string) string {
	return `{"isSidechain":false,"type":"user","timestamp":"` + timestamp + `","message":{"content":[{"tool_use_id":"tool-read","type":"tool_result","content":"ok"}]},"toolUseResult":{"type":"text","file":{"filePath":"/private/tmp/claude-501/x/tasks/` + taskID + `.output","content":"ok"}}}`
}

func TestReadBackgroundTasks_AgentsOwnReadOfTheOutputDoesNotResolve(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z"),
		readOutputLine("task-1", "2026-08-12T17:00:05.000000000Z"),
	)
	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(reading.Markers) != 1 || reading.Markers[0].Status != BackgroundTaskOutstandingFresh {
		t.Errorf("Markers = %+v, want task-1 outstanding-fresh", reading.Markers)
	}
}

// The line order of the title-bar-impl/16 incident: the agent Read-polled the
// output, ended its turn, and the notification then woke it into a new turn.
func TestReadBackgroundTasks_NotificationAfterReadPollAwaitsTurnEnd(t *testing.T) {
	start := startMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z")
	read := readOutputLine("task-1", "2026-08-12T17:00:05.000000000Z")
	waiting := assistantLine("end_turn", "2026-08-12T17:00:10.000000000Z")
	notice := notificationLine("task-1", "tool-1", "completed", "2026-08-12T17:00:14.000000000Z")
	toolCall := assistantLine("tool_use", "2026-08-12T17:00:16.000000000Z")
	for _, tc := range []struct {
		name  string
		lines []string
		want  bool
	}{
		{"turn ended after the read", []string{start, read, waiting}, false},
		{"notified", []string{start, read, waiting, notice}, true},
		{"new turn running", []string{start, read, waiting, notice, toolCall}, true},
	} {
		reading, err := ReadBackgroundTasks(writeTranscript(t, tc.lines...), capDuration, readAt)
		if err != nil {
			t.Fatal(err)
		}
		if reading.AwaitingTurnEnd != tc.want {
			t.Errorf("%s: AwaitingTurnEnd = %v, want %v", tc.name, reading.AwaitingTurnEnd, tc.want)
		}
	}
}

// A report on an already-resolved task (TaskOutput read it, then its
// notification lands after the turn ended) wakes the agent all the same.
func TestReadBackgroundTasks_LaterReportOnResolvedTaskRearms(t *testing.T) {
	path := writeTranscript(t,
		startMarkerLine("task-1", "tool-1", "2026-08-12T17:00:00.000000000Z"),
		taskOutputLine("task-1", "2026-08-12T17:00:05.000000000Z"),
		assistantLine("end_turn", "2026-08-12T17:00:10.000000000Z"),
		notificationLine("task-1", "tool-1", "completed", "2026-08-12T17:00:14.000000000Z"),
	)
	reading, err := ReadBackgroundTasks(path, capDuration, readAt)
	if err != nil {
		t.Fatal(err)
	}
	if !reading.AwaitingTurnEnd {
		t.Error("AwaitingTurnEnd = false, want true")
	}
}

// A Monitor that never reported expires on its own timeout (400s here), long
// before the 2h cap of a backgrounded shell command.
func TestReadBackgroundTasks_MonitorAgesOutAtItsOwnTimeout(t *testing.T) {
	path := writeTranscript(t, monitorStartLine("mon-1", "2026-08-12T17:00:00.000000000Z"))
	for _, tc := range []struct {
		now  string
		want BackgroundTaskStatus
	}{
		{"2026-08-12T17:07:00.000000000Z", BackgroundTaskOutstandingFresh}, // 400s + 1m not yet over
		{"2026-08-12T17:09:00.000000000Z", BackgroundTaskOutstandingAgedOut},
	} {
		reading, err := ReadBackgroundTasks(path, capDuration, mustParseTime(tc.now))
		if err != nil {
			t.Fatal(err)
		}
		if len(reading.Markers) != 1 || reading.Markers[0].Status != tc.want {
			t.Errorf("at %s: Markers = %+v, want %s", tc.now, reading.Markers, tc.want)
		}
	}
}
