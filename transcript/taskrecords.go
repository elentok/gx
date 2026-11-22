package transcript

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	exitCodeText     = regexp.MustCompile(`Exit code (\d+)`)
	summaryExitCode  = regexp.MustCompile(`exit code (\d+)`)
	outputPathText   = regexp.MustCompile(`Output is being written to: (\S+\.output)`)
	notificationKill = map[string]string{
		"completed": TaskOutcomeSuccess,
		"failed":    TaskOutcomeFailure,
		"killed":    TaskOutcomeKilled,
		"stopped":   TaskOutcomeKilled,
	}
)

// taskRecordLine is the part of a transcript line ReadTaskRecords reads.
type taskRecordLine struct {
	Type        string          `json:"type"`
	Subtype     string          `json:"subtype"`
	Operation   string          `json:"operation"`
	IsSidechain bool            `json:"isSidechain"`
	Timestamp   string          `json:"timestamp"`
	Content     string          `json:"content"` // a queue-operation's notification text
	Message     json.RawMessage `json:"message"`
	// Raw because a failed call writes a string here, not an object.
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type taskRecordResult struct {
	BackgroundTaskID string `json:"backgroundTaskId"`
	StoppedTaskID    string `json:"task_id"`
	MonitorTaskID    string `json:"taskId"`
	TimeoutMs        *int64 `json:"timeoutMs"`
	AgentID          string `json:"agentId"`
	Status           string `json:"status"`
	ResolvedModel    string `json:"resolvedModel"`
	OutputFile       string `json:"outputFile"`
	// A TaskOutput result: {retrieval_status, task: {task_id, ...}}.
	RetrievalStatus string `json:"retrieval_status"`
	Task            struct {
		TaskID string `json:"task_id"`
	} `json:"task"`
}

// ReadTaskRecords parses the parent transcript at path into task records:
// background shells, subagents and foreground calls. Sidechain lines are
// skipped. The records hold parse results only; the clock-based rules run at
// assembly. The same scan also returns the current turn's open calls, exactly
// as ReadInFlightTools lists them.
func ReadTaskRecords(path string) ([]TaskRecord, []InFlightTool, error) {
	scan, err := scanTaskRecords(path)
	if err != nil {
		return nil, nil, err
	}
	return scan.records, scan.inFlight, nil
}

// taskScan is one pass over a transcript: the task records, the open calls,
// and the line counts the background-task view needs to tell a missing or
// unreadable file from an empty one.
type taskScan struct {
	records  []TaskRecord
	inFlight []InFlightTool
	missing  bool
	nonBlank int
	parsed   int
}

func scanTaskRecords(path string) (taskScan, error) {
	var scan taskScan
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			scan.missing = true
			return scan, nil
		}
		return taskScan{}, err
	}
	defer f.Close()

	var recs []*TaskRecord
	var open inFlightTracker
	byUse := map[string]*TaskRecord{}
	byTask := map[string]*TaskRecord{}

	err = ForEachLine(f, func(b []byte) {
		if len(bytes.TrimSpace(b)) == 0 {
			return
		}
		scan.nonBlank++
		var l taskRecordLine
		if json.Unmarshal(b, &l) != nil {
			return
		}
		scan.parsed++
		if l.IsSidechain {
			return
		}
		if l.Type == "assistant" || l.Type == "user" {
			var m struct {
				Content json.RawMessage `json:"content"`
			}
			_ = json.Unmarshal(l.Message, &m)
			open.feed(l.Type, m.Content)
		}
		ts, tsErr := time.Parse(time.RFC3339Nano, l.Timestamp)
		if tsErr != nil {
			return
		}
		var res taskRecordResult
		_ = json.Unmarshal(l.ToolUseResult, &res)
		applyReports(recs, l, res, string(b), ts)
		switch l.Type {
		case "assistant":
			for id, u := range taskToolUses(l.Message) {
				r := &TaskRecord{Kind: TaskKindCall, Label: u.label(), StartedAt: ts, Command: u.Command}
				switch u.name {
				case "Monitor":
					r.Kind = TaskKindMonitor
				case "Agent", "Task":
					r.Kind = TaskKindSubagent
				default:
					if u.RunInBackground {
						r.Kind = TaskKindShell
					}
				}
				byUse[id] = r
				recs = append(recs, r)
			}
		case "system":
			if l.Subtype != "turn_duration" {
				return
			}
			for _, r := range recs {
				// A subagent with an id is async or already answered.
				foreground := r.Kind == TaskKindCall || (r.Kind == TaskKindSubagent && r.ID == "")
				if foreground && r.End == nil && r.TurnEnd.IsZero() {
					r.TurnEnd = ts
				}
			}
		case "queue-operation":
			if l.Operation == "enqueue" {
				applyNotification(l.Content, ts, byTask)
			}
		case "user":
			var text string
			var m struct {
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal(l.Message, &m) == nil && json.Unmarshal(m.Content, &text) == nil {
				applyNotification(text, ts, byTask)
				return
			}
			if r := byTask[res.StoppedTaskID]; r != nil && res.StoppedTaskID != "" && r.End == nil {
				r.End = &TaskEnd{At: ts, Outcome: TaskOutcomeKilled, Source: TaskEndTaskStop}
			}
			for _, item := range toolResults(l.Message) {
				r := byUse[item.id]
				if r == nil {
					continue
				}
				switch {
				case r.Kind == TaskKindMonitor:
					if res.TimeoutMs != nil && res.MonitorTaskID != "" {
						r.Timeout = time.Duration(*res.TimeoutMs) * time.Millisecond
						r.TaskID = res.MonitorTaskID
						byTask[res.MonitorTaskID] = r
					}
				case r.Kind == TaskKindSubagent && res.Status == "async_launched":
					r.ID, r.TaskID, r.Model = res.AgentID, res.AgentID, res.ResolvedModel
					r.OutputPath = usableOutputPath(res.OutputFile)
					if res.AgentID != "" {
						byTask[res.AgentID] = r
					}
				case res.BackgroundTaskID != "":
					r.Kind, r.TaskID = TaskKindShell, res.BackgroundTaskID
					byTask[res.BackgroundTaskID] = r
					if p := outputPathText.FindStringSubmatch(item.text); p != nil {
						r.OutputPath = p[1]
					}
				case (r.Kind == TaskKindCall || r.Kind == TaskKindSubagent) && r.End == nil:
					if res.AgentID != "" {
						r.ID = res.AgentID
					}
					end := &TaskEnd{At: ts, Outcome: TaskOutcomeSuccess, Source: TaskEndToolResult}
					if item.isError {
						end.Outcome = TaskOutcomeFailure
						if m := exitCodeText.FindStringSubmatch(item.text); m != nil {
							r.ExitCode = atoiPtr(m[1])
						}
					}
					r.End = end
				}
			}
		}
		// A launch result with no tool_use of its own in the transcript, on a
		// line of any type, still starts a background task.
		if id, kind := res.launch(); id != "" && byTask[id] == nil {
			r := &TaskRecord{Kind: kind, TaskID: id, StartedAt: ts}
			if kind == TaskKindSubagent {
				r.ID = id
			}
			if kind == TaskKindMonitor {
				r.Timeout = time.Duration(*res.TimeoutMs) * time.Millisecond
			}
			byTask[id] = r
			recs = append(recs, r)
		}
	})
	if err != nil {
		return taskScan{}, err
	}
	scan.records = make([]TaskRecord, len(recs))
	for i, r := range recs {
		scan.records[i] = *r
	}
	scan.inFlight = open.result()
	return scan, nil
}

// applyNotification ends the shell a final task-notification names. A
// notification with no status (a Monitor event) or an interim one ends nothing.
func applyNotification(text string, at time.Time, byTask map[string]*TaskRecord) {
	if !strings.Contains(text, "<task-notification>") || strings.Contains(text, interimNotificationNote) {
		return
	}
	r := byTask[tagValue(text, "task-id")]
	if r == nil {
		return
	}
	outcome, ok := notificationKill[tagValue(text, "status")]
	if !ok {
		// A Monitor event notice. The expiry notice lands after the timeout, so
		// the time test leaves it out, not its text.
		if r.Kind == TaskKindMonitor && r.Timeout > 0 && at.Before(r.StartedAt.Add(r.Timeout)) {
			r.EventCount++
			r.LastEventAt = at
		}
		return
	}
	if p := usableOutputPath(tagValue(text, "output-file")); p != "" {
		r.OutputPath = p
	}
	if r.Kind == TaskKindSubagent {
		tokens, tokErr := strconv.Atoi(tagValue(text, "subagent_tokens"))
		calls, callErr := strconv.Atoi(tagValue(text, "tool_uses"))
		if tokErr == nil && callErr == nil {
			r.Tokens, r.ToolCalls, r.UsageFinal = tokens, calls, true
		}
	}
	if r.End != nil && r.End.Source == TaskEndNotification {
		return
	}
	r.End = &TaskEnd{At: at, Outcome: outcome, Source: TaskEndNotification}
	if m := summaryExitCode.FindStringSubmatch(tagValue(text, "summary")); m != nil {
		r.ExitCode = atoiPtr(m[1])
	}
}

func atoiPtr(s string) *int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

// tagValue is the trimmed text of the first <tag>...</tag> in s.
func tagValue(s, tag string) string {
	_, rest, ok := strings.Cut(s, "<"+tag+">")
	if !ok {
		return ""
	}
	v, _, _ := strings.Cut(rest, "</"+tag+">")
	return strings.TrimSpace(v)
}

// usableOutputPath drops the directory-only path some results carry.
func usableOutputPath(p string) string {
	if strings.HasSuffix(p, "/") {
		return ""
	}
	return p
}

type taskUse struct {
	toolUseInput
	name            string
	RunInBackground bool `json:"run_in_background"`
}

// taskToolUses reads the Bash, Monitor and Agent (or legacy Task) launches of
// an assistant message by tool_use id.
func taskToolUses(message json.RawMessage) map[string]taskUse {
	var m struct {
		Content []struct {
			Type  string  `json:"type"`
			ID    string  `json:"id"`
			Name  string  `json:"name"`
			Input taskUse `json:"input"`
		} `json:"content"`
	}
	if json.Unmarshal(message, &m) != nil {
		return nil
	}
	out := map[string]taskUse{}
	for _, c := range m.Content {
		switch c.Name {
		case "Bash", "Monitor", "Agent", "Task":
			if c.Type == "tool_use" {
				c.Input.name = c.Name
				out[c.ID] = c.Input
			}
		}
	}
	return out
}

type toolResultItem struct {
	id      string
	text    string
	isError bool
}

func toolResults(message json.RawMessage) []toolResultItem {
	var m struct {
		Content []struct {
			Type      string          `json:"type"`
			ToolUseID string          `json:"tool_use_id"`
			IsError   bool            `json:"is_error"`
			Content   json.RawMessage `json:"content"`
		} `json:"content"`
	}
	if json.Unmarshal(message, &m) != nil {
		return nil
	}
	var out []toolResultItem
	for _, c := range m.Content {
		if c.Type != "tool_result" {
			continue
		}
		var text string
		if json.Unmarshal(c.Content, &text) != nil {
			var parts []struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(c.Content, &parts)
			for _, p := range parts {
				text += p.Text + "\n"
			}
		}
		out = append(out, toolResultItem{id: c.ToolUseID, text: text, isError: c.IsError})
	}
	return out
}
