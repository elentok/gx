package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
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

// BackgroundTaskMarker is one non-sidechain backgrounded-shell-command start
// marker found in a transcript, together with its resolution state.
type BackgroundTaskMarker struct {
	TaskID    string
	Status    BackgroundTaskStatus
	StartedAt time.Time
}

// BackgroundTaskReading is ReadBackgroundTasks' result. FileStatus is set to
// BackgroundTaskUnreadable or BackgroundTaskUnsupported (with Markers left
// nil) when the read failed at the whole-file level; it is the zero value
// ("") otherwise, and callers should inspect Markers instead — including
// when Markers is empty, which just means no background task was found.
type BackgroundTaskReading struct {
	Markers    []BackgroundTaskMarker
	FileStatus BackgroundTaskStatus
}

// backgroundTaskLine is the subset of a transcript JSONL line
// ReadBackgroundTasks reads: a tool_result line's backgroundTaskId (the
// start marker, found in toolUseResult, with the triggering tool_use_id
// inside message.content) and a task-notification line's origin.kind (whose
// <task-id>/<tool-use-id> live inside message.content as an XML-ish string,
// not structured JSON).
type backgroundTaskLine struct {
	IsSidechain bool   `json:"isSidechain"`
	Timestamp   string `json:"timestamp"`
	Origin      struct {
		Kind string `json:"kind"`
	} `json:"origin"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	ToolUseResult struct {
		BackgroundTaskID string `json:"backgroundTaskId"`
		RetrievalStatus  string `json:"retrieval_status"`
		Task             struct {
			TaskID string `json:"task_id"`
		} `json:"task"`
	} `json:"toolUseResult"`
}

// toolResultContentItem is message.content's shape on a start-marker line: a
// one-element array carrying the tool_use_id the backgroundTaskId resolves.
type toolResultContentItem struct {
	ToolUseID string `json:"tool_use_id"`
}

var (
	taskIDTagRe    = regexp.MustCompile(`<task-id>(.*?)</task-id>`)
	toolUseIDTagRe = regexp.MustCompile(`<tool-use-id>(.*?)</tool-use-id>`)
)

// ReadBackgroundTasks scans the transcript at path for non-sidechain
// backgrounded-shell-command start markers (a tool_result carrying
// backgroundTaskId) and, for each, whether it was later resolved — either by
// a passive task-notification transcript entry, or by a TaskOutput tool call
// blocking on the same task id and returning its result inline (which never
// produces a task-notification entry of its own). Sidechain-scoped markers
// (isSidechain: true) are excluded entirely — a subagent's background task
// belongs to the subagent's lifetime, never the parent iteration's. Either
// resolution whose task id never had a matching start marker is ignored.
//
// now is compared against each marker's own timestamp to decide
// outstanding-fresh vs outstanding-aged-out against cap — passed explicitly
// (rather than time.Now()) so callers and tests get a deterministic read.
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
	}
	markers := map[string]*markerState{}
	var order []string
	toolUseToTaskID := map[string]string{}

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

		switch {
		case entry.ToolUseResult.BackgroundTaskID != "":
			if entry.IsSidechain {
				continue
			}
			taskID := entry.ToolUseResult.BackgroundTaskID
			ts, tsErr := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if tsErr != nil {
				continue
			}
			if _, exists := markers[taskID]; !exists {
				markers[taskID] = &markerState{taskID: taskID, startedAt: ts}
				order = append(order, taskID)
			}
			var items []toolResultContentItem
			if json.Unmarshal(entry.Message.Content, &items) == nil && len(items) > 0 && items[0].ToolUseID != "" {
				toolUseToTaskID[items[0].ToolUseID] = taskID
			}

		case entry.Origin.Kind == "task-notification":
			var content string
			if json.Unmarshal(entry.Message.Content, &content) != nil {
				continue
			}
			taskID := firstSubmatch(taskIDTagRe, content)
			if taskID == "" {
				if toolUseID := firstSubmatch(toolUseIDTagRe, content); toolUseID != "" {
					taskID = toolUseToTaskID[toolUseID]
				}
			}
			if m, ok := markers[taskID]; ok {
				m.resolved = true
			}

		case entry.ToolUseResult.RetrievalStatus != "" && entry.ToolUseResult.Task.TaskID != "":
			// A TaskOutput tool call blocks on the backgrounded task and
			// returns its result inline, instead of the task's completion
			// ever landing as a passive task-notification transcript entry —
			// so a marker only ever retrieved this way would otherwise never
			// resolve and hold the gate until the aged-out cap.
			if m, ok := markers[entry.ToolUseResult.Task.TaskID]; ok {
				m.resolved = true
			}
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return BackgroundTaskReading{}, scanErr
	}
	if totalNonBlank > 0 && parsedOK == 0 {
		return BackgroundTaskReading{FileStatus: BackgroundTaskUnreadable}, nil
	}

	reading := BackgroundTaskReading{}
	for _, taskID := range order {
		m := markers[taskID]
		status := BackgroundTaskOutstandingFresh
		switch {
		case m.resolved:
			status = BackgroundTaskResolved
		case now.Sub(m.startedAt) > cap:
			status = BackgroundTaskOutstandingAgedOut
		}
		reading.Markers = append(reading.Markers, BackgroundTaskMarker{
			TaskID:    taskID,
			Status:    status,
			StartedAt: m.startedAt,
		})
	}
	return reading, nil
}

func firstSubmatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}
