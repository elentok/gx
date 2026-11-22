package transcript

import (
	"encoding/json"
	"strings"
	"time"
)

// interimNotificationNote marks a task-notification for a subagent that ended
// its turn still waiting on background work of its own: the same id notifies
// again when it really finishes, so this one must not resolve the task.
const interimNotificationNote = "the result below may be interim"

// launch reads the background task a tool result starts: a shell
// (backgroundTaskId), an async Agent (status async_launched) or a Monitor
// (taskId plus timeoutMs). The id is empty when the result starts none.
func (res taskRecordResult) launch() (id, kind string) {
	if res.BackgroundTaskID != "" {
		return res.BackgroundTaskID, TaskKindShell
	}
	if res.Status == "async_launched" {
		id, kind = res.AgentID, TaskKindSubagent
	}
	if id == "" && res.TimeoutMs != nil {
		id, kind = res.MonitorTaskID, TaskKindMonitor
	}
	return id, kind
}

// structuredTaskID is the one task id a TaskStop or TaskOutput tool_result
// names in its structured fields. These results report only by that id, never
// by a raw-line match: a TaskStop of the agent's own poll loop echoes a live
// task's id in its message and command.
func (res taskRecordResult) structuredTaskID() string {
	if res.Task.TaskID != "" {
		return res.Task.TaskID
	}
	return res.StoppedTaskID
}

// hasToolResult reports whether a user message carries a tool_result item.
func hasToolResult(message json.RawMessage) bool {
	return len(toolResults(message)) > 0
}

// reportsOnTasks reports whether a line is the CLI reporting on a task. It is
// an allowlist: any other line type, including ones the CLI adds later, can
// echo the id (an api-request-blob replaying the agent's own `cat` of the
// task's .output file) while the task still runs. A tool_result counts only
// from TaskOutput or TaskStop, for the same reason.
func (l taskRecordLine) reportsOnTasks(res taskRecordResult) bool {
	switch l.Type {
	case "queue-operation", "attachment":
		return true
	case "user":
	default:
		return false
	}
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(l.Message, &m) != nil {
		return false
	}
	var text string
	if json.Unmarshal(m.Content, &text) == nil {
		return strings.Contains(text, "<task-notification>")
	}
	if hasToolResult(l.Message) {
		return res.RetrievalStatus != "" || res.StoppedTaskID != ""
	}
	return false
}

// applyReports records a line's effect on the background tasks launched so
// far. A task id on a report line marks the task reported; a report on an
// already reported task still wakes the agent, so it re-arms AwaitingTurnEnd.
// An assistant turn end clears it.
func applyReports(recs []*TaskRecord, l taskRecordLine, res taskRecordResult, raw string, at time.Time) {
	if l.Type == "assistant" && endsTurn(l.Message) {
		for _, r := range recs {
			r.AwaitingTurnEnd = false
		}
	}
	if !l.reportsOnTasks(res) || strings.Contains(raw, interimNotificationNote) {
		return
	}
	byStructuredID := hasToolResult(l.Message)
	for _, r := range recs {
		if r.TaskID == "" {
			continue
		}
		if byStructuredID && res.structuredTaskID() != r.TaskID || !byStructuredID && !containsTaskIDToken(raw, r.TaskID) {
			continue
		}
		r.Reported, r.ReportedAt, r.AwaitingTurnEnd = true, at, true
	}
}

// containsTaskIDToken reports whether taskID appears in raw as its own
// token: the characters immediately flanking every candidate match must not
// themselves be id-token characters (alphanumeric/_/-), so a shorter id
// (task-1) doesn't spuriously match inside a longer prefix-colliding one
// (task-10, task-100).
func containsTaskIDToken(raw, taskID string) bool {
	for searchFrom := 0; ; {
		i := strings.Index(raw[searchFrom:], taskID)
		if i < 0 {
			return false
		}
		start := searchFrom + i
		end := start + len(taskID)
		beforeOK := start == 0 || !isIDTokenChar(raw[start-1])
		afterOK := end == len(raw) || !isIDTokenChar(raw[end])
		if beforeOK && afterOK {
			return true
		}
		searchFrom = start + 1
	}
}

func isIDTokenChar(b byte) bool {
	return b == '_' || b == '-' ||
		(b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}

// endsTurn reports whether an assistant message closes the agent's turn: any
// stop_reason except tool_use (streamed partial lines carry none).
func endsTurn(message json.RawMessage) bool {
	var m struct {
		StopReason string `json:"stop_reason"`
	}
	if json.Unmarshal(message, &m) != nil {
		return false
	}
	return m.StopReason != "" && m.StopReason != "tool_use"
}
