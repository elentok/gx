package transcript

import (
	"bufio"
	"encoding/json"
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
// ReadBackgroundTasks reads to find a start marker: a tool_result line's
// backgroundTaskId, found in toolUseResult. Resolution isn't matched against
// any particular field — see ReadBackgroundTasks — so nothing else needs
// parsing.
type backgroundTaskLine struct {
	Type          string `json:"type"`
	IsSidechain   bool   `json:"isSidechain"`
	Timestamp     string `json:"timestamp"`
	ToolUseResult struct {
		BackgroundTaskID string `json:"backgroundTaskId"`
		// A Monitor start result: {taskId, timeoutMs, persistent}. The agent
		// that ends its turn waiting on one is not finished either.
		MonitorTaskID string `json:"taskId"`
		TimeoutMs     *int   `json:"timeoutMs"`
	} `json:"toolUseResult"`
}

// ReadBackgroundTasks scans the transcript at path for non-sidechain
// backgrounded-shell-command start markers (a tool_result carrying
// backgroundTaskId) and, for each, whether it was later resolved. A marker
// resolves as soon as its task id string appears anywhere on a later
// non-sidechain transcript line — regardless of that line's
// type/origin/JSON nesting — because the CLI necessarily refers to a task by
// its id wherever it reports on it, whatever shape that report takes.
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
		cap time.Duration
	}
	markers := map[string]*markerState{}
	var order []string

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

		// The agent's own tool calls name its task (a Monitor on the output
		// path, say) without the task having finished: only what the CLI
		// reports back can resolve it.
		for _, taskID := range order {
			m := markers[taskID]
			if !m.resolved && entry.Type != "assistant" && containsTaskIDToken(raw, taskID) {
				m.resolved = true
			}
		}

		taskID, markerCap := entry.ToolUseResult.BackgroundTaskID, cap
		if taskID == "" && entry.ToolUseResult.TimeoutMs != nil {
			taskID = entry.ToolUseResult.MonitorTaskID
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
				markers[taskID] = &markerState{taskID: taskID, startedAt: ts, cap: markerCap}
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

	reading := BackgroundTaskReading{}
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
		})
	}
	return reading, nil
}
