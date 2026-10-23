package transcript

import (
	"bufio"
	"encoding/json"
	"maps"
	"os"
	"strings"
	"time"
)

// BackgroundTaskStatus is one of the five states ReadBackgroundTasks can
// report, either as a marker's own resolution state or (Unreadable/
// Unsupported only) as BackgroundTaskReading.FileStatus when the whole read
// failed before any marker could be resolved.
type BackgroundTaskStatus string

const (
	// BackgroundTaskResolved means a task-notification matching the marker's
	// task id was found later in the transcript. Its reported <status>
	// (completed/failed/killed) doesn't matter — only that one exists.
	BackgroundTaskResolved BackgroundTaskStatus = "resolved"
	// BackgroundTaskOutstandingFresh means no matching notification has
	// landed yet, and the marker is younger than the cap passed to
	// ReadBackgroundTasks.
	BackgroundTaskOutstandingFresh BackgroundTaskStatus = "outstanding-fresh"
	// BackgroundTaskOutstandingAgedOut means no matching notification has
	// landed, and the marker has exceeded the cap. Neutral: callers must not
	// treat this as evidence of anything, gating or disproving.
	BackgroundTaskOutstandingAgedOut BackgroundTaskStatus = "outstanding-aged-out"
	// BackgroundTaskUnreadable means the transcript file exists but every
	// non-blank line in it failed to parse as JSON.
	BackgroundTaskUnreadable BackgroundTaskStatus = "unreadable"
	// BackgroundTaskUnsupported means there is no transcript file at path at
	// all (e.g. a Codex session, which has no Claude-style transcript).
	BackgroundTaskUnsupported BackgroundTaskStatus = "unsupported"
)

// BackgroundTaskAgedOutCap is the cap gx's callers pass to
// ReadBackgroundTasks: how long an outstanding marker is trusted as evidence
// the agent isn't really finished. Past it, the background job is assumed
// abandoned or unresolvable rather than waited on forever.
const BackgroundTaskAgedOutCap = 2 * time.Hour

// BackgroundTaskMarker is one non-sidechain start marker (a backgrounded shell
// command, Monitor or async Agent) found in a transcript, together with its
// resolution state.
type BackgroundTaskMarker struct {
	TaskID    string
	Status    BackgroundTaskStatus
	StartedAt time.Time
	// Cap is how long the marker may stay outstanding before it ages out.
	Cap time.Duration
	// Kind is BackgroundTaskShell, BackgroundTaskMonitor or BackgroundTaskAgent.
	Kind string
	// Label is the launching tool call's description, else the first
	// backgroundLabelLen characters of its command.
	Label string
}

// Kinds of a BackgroundTaskMarker.
const (
	BackgroundTaskShell   = "shell"
	BackgroundTaskMonitor = "monitor"
	BackgroundTaskAgent   = "agent"
)

const backgroundLabelLen = 60

// toolUseInput is the part of a tool_use block that names what it launched.
type toolUseInput struct {
	Description string `json:"description"`
	Command     string `json:"command"`
}

func (in toolUseInput) label() string {
	if in.Description != "" {
		return in.Description
	}
	return truncateRunes(in.Command, backgroundLabelLen)
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// toolUses reads the tool_use inputs of an assistant message by tool_use id.
func toolUses(message json.RawMessage) map[string]toolUseInput {
	var m struct {
		Content []struct {
			Type  string       `json:"type"`
			ID    string       `json:"id"`
			Input toolUseInput `json:"input"`
		} `json:"content"`
	}
	if json.Unmarshal(message, &m) != nil {
		return nil
	}
	out := map[string]toolUseInput{}
	for _, c := range m.Content {
		if c.Type == "tool_use" {
			out[c.ID] = c.Input
		}
	}
	return out
}

// toolResultIDs lists the tool_use ids a user message answers.
func toolResultIDs(message json.RawMessage) []string {
	var m struct {
		Content []struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
		} `json:"content"`
	}
	if json.Unmarshal(message, &m) != nil {
		return nil
	}
	var ids []string
	for _, c := range m.Content {
		if c.Type == "tool_result" {
			ids = append(ids, c.ToolUseID)
		}
	}
	return ids
}

// BackgroundTaskReading is ReadBackgroundTasks' result. FileStatus is set to
// BackgroundTaskUnreadable or BackgroundTaskUnsupported (with Markers left
// nil) when the read failed at the whole-file level; it is the zero value
// ("") otherwise, and callers should inspect Markers instead — including
// when Markers is empty, which just means no background task was found.
type BackgroundTaskReading struct {
	Markers    []BackgroundTaskMarker
	FileStatus BackgroundTaskStatus
	// AwaitingTurnEnd is true when the latest line resolving a marker has no
	// assistant turn end after it yet. The resolving notification wakes an
	// agent that ended its turn to wait, and the pane can still read idle
	// while that new turn runs short tool calls. Past cap it reads false.
	AwaitingTurnEnd bool
}

// backgroundTaskLine is the subset of a transcript JSONL line
// ReadBackgroundTasks reads to find a start marker: a tool_result line's
// backgroundTaskId, found in toolUseResult. Resolution isn't matched against
// any particular field — see ReadBackgroundTasks — so nothing else needs
// parsing.
type backgroundTaskLine struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Timestamp   string `json:"timestamp"`
	// Raw so an unexpected message shape can't fail the whole line's parse.
	Message       json.RawMessage `json:"message"`
	ToolUseResult struct {
		BackgroundTaskID string `json:"backgroundTaskId"`
		// A Monitor start result: {taskId, timeoutMs, persistent}. The agent
		// that ends its turn waiting on one is not finished either.
		MonitorTaskID string `json:"taskId"`
		TimeoutMs     *int   `json:"timeoutMs"`
		// An async Agent launch: {isAsync, status: "async_launched", agentId}.
		Status  string `json:"status"`
		AgentID string `json:"agentId"`
		// A TaskOutput result: {retrieval_status, task: {task_id, ...}}.
		RetrievalStatus string `json:"retrieval_status"`
		// A TaskStop result: {message, task_id}.
		StoppedTaskID string `json:"task_id"`
	} `json:"toolUseResult"`
}

// reportsOnTasks reports whether a line is the CLI reporting on a task. It is
// an allowlist: any other line type, including ones the CLI adds later, can
// echo the id (an api-request-blob replaying the agent's own `cat` of the
// task's .output file) while the task still runs. A tool_result counts only
// from TaskOutput or TaskStop, for the same reason.
func (l backgroundTaskLine) reportsOnTasks() bool {
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
	var items []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(m.Content, &items) != nil {
		return false
	}
	for _, c := range items {
		if c.Type == "tool_result" {
			return l.ToolUseResult.RetrievalStatus != "" || l.ToolUseResult.StoppedTaskID != ""
		}
	}
	return false
}

// interimNotificationNote marks a task-notification for a subagent that ended
// its turn still waiting on background work of its own: the same id notifies
// again when it really finishes, so this one must not resolve the marker.
const interimNotificationNote = "the result below may be interim"

// ReadBackgroundTasks scans the transcript at path for non-sidechain
// backgrounded-shell-command start markers (a tool_result carrying
// backgroundTaskId) and, for each, whether it was later resolved. A marker
// resolves as soon as its task id string appears anywhere on a later
// non-sidechain line the CLI wrote to report on tasks (see reportsOnTasks) —
// regardless of that line's JSON nesting — because the CLI necessarily refers
// to a task by its id wherever it reports on it, whatever shape that report
// takes.
// Sidechain-scoped markers (isSidechain: true) are excluded entirely, both
// as start markers and as resolution lines — a subagent's background task
// belongs to the subagent's lifetime, never the parent iteration's, so a
// sidechain line can never resolve a parent marker either.
//
// now is compared against each marker's own timestamp to decide
// outstanding-fresh vs outstanding-aged-out against cap — passed explicitly
// (rather than time.Now()) so callers and tests get a deterministic read.
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

func isIDTokenChar(b byte) bool {
	return b == '_' || b == '-' ||
		(b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}

func ReadBackgroundTasks(path string, cap time.Duration, now time.Time) (BackgroundTaskReading, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BackgroundTaskReading{FileStatus: BackgroundTaskUnsupported}, nil
		}
		return BackgroundTaskReading{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	type markerState struct {
		taskID    string
		startedAt time.Time
		resolved  bool
		// cap is how long this marker may stay outstanding before it ages out.
		cap   time.Duration
		kind  string
		label string
	}
	launches := map[string]toolUseInput{}
	markers := map[string]*markerState{}
	var order []string
	awaitingTurnEnd := false
	var lastResolvedAt time.Time

	totalNonBlank := 0
	parsedOK := 0

	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		totalNonBlank++

		var entry backgroundTaskLine
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			continue
		}
		parsedOK++

		if entry.IsSidechain {
			continue
		}

		// A report on an already-resolved task
		// (its notification after a TaskOutput read) still wakes the agent,
		// so it re-arms AwaitingTurnEnd too.
		if entry.reportsOnTasks() && !strings.Contains(raw, interimNotificationNote) {
			for _, taskID := range order {
				m := markers[taskID]
				if containsTaskIDToken(raw, taskID) {
					m.resolved = true
					awaitingTurnEnd = true
					lastResolvedAt, _ = time.Parse(time.RFC3339Nano, entry.Timestamp)
				}
			}
		}
		if entry.Type == "assistant" && endsTurn(entry.Message) {
			awaitingTurnEnd = false
		}

		if entry.Type == "assistant" {
			maps.Copy(launches, toolUses(entry.Message))
		}

		taskID, markerCap, kind := entry.ToolUseResult.BackgroundTaskID, cap, BackgroundTaskShell
		if taskID == "" && entry.ToolUseResult.Status == "async_launched" {
			taskID, kind = entry.ToolUseResult.AgentID, BackgroundTaskAgent
		}
		if taskID == "" && entry.ToolUseResult.TimeoutMs != nil {
			taskID, kind = entry.ToolUseResult.MonitorTaskID, BackgroundTaskMonitor
			// A Monitor expires on its own timeout; waiting longer than that
			// (plus a minute for its notice) only strands a finished agent.
			markerCap = min(cap, time.Duration(*entry.ToolUseResult.TimeoutMs)*time.Millisecond+time.Minute)
		}
		if taskID != "" {
			ts, tsErr := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if tsErr != nil {
				continue
			}
			if _, exists := markers[taskID]; !exists {
				label := ""
				for _, id := range toolResultIDs(entry.Message) {
					if in, ok := launches[id]; ok {
						label = in.label()
						break
					}
				}
				markers[taskID] = &markerState{taskID: taskID, startedAt: ts, cap: markerCap, kind: kind, label: label}
				order = append(order, taskID)
			}
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return BackgroundTaskReading{}, scanErr
	}
	if totalNonBlank > 0 && parsedOK == 0 {
		return BackgroundTaskReading{FileStatus: BackgroundTaskUnreadable}, nil
	}

	reading := BackgroundTaskReading{AwaitingTurnEnd: awaitingTurnEnd && now.Sub(lastResolvedAt) <= cap}
	for _, taskID := range order {
		m := markers[taskID]
		status := BackgroundTaskOutstandingFresh
		switch {
		case m.resolved:
			status = BackgroundTaskResolved
		case now.Sub(m.startedAt) > m.cap:
			status = BackgroundTaskOutstandingAgedOut
		}
		reading.Markers = append(reading.Markers, BackgroundTaskMarker{
			TaskID:    taskID,
			Status:    status,
			StartedAt: m.startedAt,
			Cap:       m.cap,
			Kind:      m.kind,
			Label:     m.label,
		})
	}
	return reading, nil
}
