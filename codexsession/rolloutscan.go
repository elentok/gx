package codexsession

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/transcript"
)

// Rollout is what one scan of a rollout yields: the task records and the tool
// calls still in flight. One scan feeds both, so an end that one learns (an
// item_completed line) is the other's too.
type Rollout struct {
	Records  []transcript.TaskRecord
	InFlight []transcript.InFlightTool
}

// ReadRollout scans the rollout once. Records are filled only with execShape (a
// rollout of Codex 0.144.1 or later). A missing file is an empty rollout.
func ReadRollout(path string, execShape bool) (Rollout, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Rollout{InFlight: []transcript.InFlightTool{}}, nil
		}
		return Rollout{}, err
	}
	defer f.Close()

	s := &scanner{
		execShape: execShape,
		byCall:    map[string]*cell{},
		byCellID:  map[string]*cell{},
		byWait:    map[string]*cell{},
		bySession: map[string]*cell{},

		pollSession: map[string]string{},
		spawns:      map[string]*subagent{},
		byThread:    map[string]*subagent{},
		agentWaits:  map[string]bool{},

		yielded: map[string]string{},
		waits:   map[string]string{},
	}
	if err := transcript.ForEachLine(f, s.line); err != nil {
		return Rollout{}, err
	}
	out := Rollout{InFlight: make([]transcript.InFlightTool, 0, len(s.calls))}
	for _, c := range slices.Backward(s.calls) {
		out.InFlight = append(out.InFlight, c.tool)
	}
	if execShape {
		for _, c := range s.cells {
			out.Records = append(out.Records, *c.rec)
		}
		for _, a := range s.subagents {
			out.Records = append(out.Records, *a.rec)
		}
		slices.SortStableFunc(out.Records, func(a, b transcript.TaskRecord) int { return a.StartedAt.Compare(b.StartedAt) })
	}
	return out, nil
}

// cell is one exec cell that runs a command.
type cell struct {
	callID   string
	rec      *transcript.TaskRecord
	commands int
	// processID is the session_id the cell returned for a process that outlived it.
	processID string
	// yielded is set once the cell returned "Script running with cell ID N".
	yielded bool
}

// subagent is one spawn_agent call.
type subagent struct {
	rec       *transcript.TaskRecord
	agentPath string
}

type openCall struct {
	id   string
	tool transcript.InFlightTool
}

type scanner struct {
	execShape bool

	// Task state.
	cells       []*cell
	byCall      map[string]*cell
	byCellID    map[string]*cell
	byWait      map[string]*cell
	bySession   map[string]*cell
	pollSession map[string]string // call id of a write_stdin cell -> its session id
	subagents   []*subagent
	spawns      map[string]*subagent // spawn_agent call id
	byThread    map[string]*subagent
	agentWaits  map[string]bool // call ids of wait_agent / list_agents

	// In-flight state, reset by each task_started.
	calls   []openCall
	yielded map[string]string // cell id -> call id of the yielded exec cell
	waits   map[string]string // wait call id -> call id of the cell it polls
}

// completedItem is the item of an item_completed event: a CommandExecution or a
// SubAgentActivity. aggregated_output can be 64 KiB, so only item lines decode it.
type completedItem struct {
	Type             string   `json:"type"`
	ID               string   `json:"id"`
	ProcessID        string   `json:"process_id"`
	Command          []string `json:"command"`
	ExitCode         *int     `json:"exit_code"`
	AggregatedOutput string   `json:"aggregated_output"`
	Kind             string   `json:"kind"`
	AgentThreadID    string   `json:"agent_thread_id"`
	AgentPath        string   `json:"agent_path"`
	Model            string   `json:"model"`
}

var itemCompletedMarker = []byte(`"item_completed"`)

func (s *scanner) line(raw []byte) {
	var line chatRolloutLine
	if json.Unmarshal(raw, &line) != nil {
		return
	}
	s.inflight(line)
	if !s.execShape {
		return
	}
	ts, ok := parseRolloutTime(line.Timestamp)
	if !ok {
		return
	}
	switch {
	case line.Type == "event_msg" && line.Payload.Type == "turn_aborted":
		s.abortTurn(ts)
	case line.Type == "response_item":
		s.task(line, ts)
	case line.Type == "event_msg" && line.Payload.Type == "item_completed" && bytes.Contains(raw, itemCompletedMarker):
		var ev struct {
			Payload struct {
				Item        completedItem `json:"item"`
				StartedAtMS int64         `json:"started_at_ms"`
			} `json:"payload"`
		}
		if json.Unmarshal(raw, &ev) != nil {
			return
		}
		s.itemCompleted(ev.Payload.Item, ev.Payload.StartedAtMS, ts)
	}
}

func (s *scanner) endCell(c *cell, at time.Time, outcome string, code *int) {
	if c.rec.End != nil {
		return
	}
	if code != nil {
		outcome = transcript.TaskOutcomeSuccess
		if *code != 0 {
			outcome = transcript.TaskOutcomeFailure
		}
	}
	c.rec.End = &transcript.TaskEnd{At: at, Outcome: outcome, Source: transcript.TaskEndToolResult}
	c.rec.ExitCode = code
	// The row ended, so the Working row must stop listing its call.
	s.dropCall(c.callID)
}

// abortTurn ends the cells that have no output yet when their turn aborts (a
// call waiting for approval never gets one). A yielded cell is not ended: its
// process may keep running in the background.
func (s *scanner) abortTurn(at time.Time) {
	for _, c := range s.cells {
		if !c.yielded && c.processID == "" {
			s.endCell(c, at, transcript.TaskOutcomeKilled, nil)
		}
	}
}

func (s *scanner) dropCall(id string) {
	s.calls = slices.DeleteFunc(s.calls, func(c openCall) bool { return c.id == id })
}

func (s *scanner) task(line chatRolloutLine, ts time.Time) {
	p := line.Payload
	switch p.Type {
	case "custom_tool_call":
		if p.Name != "exec" {
			return
		}
		if strings.Contains(p.Input, execCall) {
			command := execCommand(p.Input)
			rec := &transcript.TaskRecord{
				Kind:      transcript.TaskKindShell,
				Label:     transcript.NewInFlightTool("exec_command", map[string]any{"cmd": command}).Label,
				StartedAt: ts,
				Command:   command,
			}
			c := &cell{callID: p.CallID, rec: rec, commands: strings.Count(p.Input, execCall)}
			s.cells = append(s.cells, c)
			s.byCall[p.CallID] = c
		} else if strings.Count(p.Input, writeStdinCall) == 1 {
			if m := sessionRE.FindStringSubmatch(p.Input); m != nil {
				s.pollSession[p.CallID] = m[1]
			}
		}
	case "custom_tool_call_output":
		out := parseCellOutput(p.Output)
		if c := s.byCall[p.CallID]; c != nil {
			if id, ok := out.cellID(); ok {
				s.byCellID[id] = c
				c.yielded = true
				return
			}
			outcome, ended := out.scriptEnd()
			if !ended {
				return
			}
			code := out.exitCode()
			if sid, ok := out.sessionID(); ok && code == nil {
				s.bySession[sid] = c
				c.processID = sid
				return
			}
			if c.commands != 1 {
				code = nil
			}
			s.endCell(c, ts, outcome, code)
		} else if sid, ok := s.pollSession[p.CallID]; ok {
			_, stillAlive := out.sessionID()
			if code := out.exitCode(); code != nil && !stillAlive && s.bySession[sid] != nil {
				s.endCell(s.bySession[sid], ts, "", code)
			}
		}
	case "function_call":
		switch p.Name {
		case "wait":
			if c := s.byCellID[waitCellID(p.Arguments)]; c != nil {
				s.byWait[p.CallID] = c
			}
		case "spawn_agent":
			var a struct {
				TaskName string `json:"task_name"`
			}
			_ = json.Unmarshal([]byte(p.Arguments), &a)
			rec := &transcript.TaskRecord{Kind: transcript.TaskKindSubagent, Label: a.TaskName, StartedAt: ts}
			sub := &subagent{rec: rec}
			s.subagents = append(s.subagents, sub)
			s.spawns[p.CallID] = sub
		case "wait_agent", "list_agents":
			s.agentWaits[p.CallID] = true
		}
	case "function_call_output":
		if c := s.byWait[p.CallID]; c != nil {
			if outcome, ended := parseCellOutput(p.Output).scriptEnd(); ended {
				s.endCell(c, ts, outcome, nil)
			}
		}
		if s.agentWaits[p.CallID] {
			s.agentsCompleted(p.Output, ts)
		}
	}
}

func (s *scanner) endSubagent(a *subagent, at time.Time) {
	if a.rec.End == nil {
		a.rec.End = &transcript.TaskEnd{At: at, Source: transcript.TaskEndToolResult}
	}
}

// agentsCompleted ends the subagents a wait_agent or list_agents output names as
// completed, in either of its two shapes.
func (s *scanner) agentsCompleted(output json.RawMessage, at time.Time) {
	var text string
	if json.Unmarshal(output, &text) != nil {
		return
	}
	var body struct {
		Status map[string]map[string]json.RawMessage `json:"status"`
		Agents []struct {
			AgentName   string                     `json:"agent_name"`
			AgentStatus map[string]json.RawMessage `json:"agent_status"`
		} `json:"agents"`
	}
	if json.Unmarshal([]byte(text), &body) != nil {
		return
	}
	for id, status := range body.Status {
		if _, done := status["completed"]; done && s.byThread[id] != nil {
			s.endSubagent(s.byThread[id], at)
		}
	}
	for _, ag := range body.Agents {
		if _, done := ag.AgentStatus["completed"]; !done || ag.AgentName == "" {
			continue
		}
		for _, a := range s.subagents {
			if a.agentPath == ag.AgentName {
				s.endSubagent(a, at)
			}
		}
	}
}

func (s *scanner) itemCompleted(it completedItem, startedAtMS int64, ts time.Time) {
	switch it.Type {
	case "CommandExecution":
		if it.ExitCode == nil || len(it.Command) == 0 {
			return
		}
		c := s.itemRow(it, startedAtMS, ts)
		if c == nil {
			return
		}
		s.endCell(c, ts, "", it.ExitCode)
		c.rec.OutputBytes = int64(len(it.AggregatedOutput))
		c.rec.LastLine = lastOutputLine(it.AggregatedOutput)
	case "SubAgentActivity":
		switch it.Kind {
		case "started":
			if a := s.spawns[it.ID]; a != nil && it.AgentThreadID != "" {
				a.rec.ID, a.agentPath = it.AgentThreadID, it.AgentPath
				a.rec.Model = it.Model
				s.byThread[it.AgentThreadID] = a
			}
		case "completed":
			if a := s.byThread[it.AgentThreadID]; a != nil {
				s.endSubagent(a, ts)
			}
		}
	}
}

// itemRow finds the row a CommandExecution item ended. The item has no call id:
// the process id matches a cell's returned session id exactly; otherwise the
// last argv element matches the command of the latest-starting open row that
// has no process id and started no later than a second after the item did.
func (s *scanner) itemRow(it completedItem, startedAtMS int64, ts time.Time) *cell {
	if c := s.bySession[it.ProcessID]; it.ProcessID != "" && c != nil {
		if c.rec.End != nil || c.commands != 1 {
			return nil
		}
		return c
	}
	latest := ts
	if startedAtMS > 0 {
		latest = time.UnixMilli(startedAtMS)
	}
	latest = latest.Add(time.Second)
	command := it.Command[len(it.Command)-1]
	var found *cell
	for _, c := range s.cells {
		if c.rec.End != nil || c.processID != "" || c.commands != 1 || c.rec.Command != command || c.rec.StartedAt.After(latest) {
			continue
		}
		if found == nil || !c.rec.StartedAt.Before(found.rec.StartedAt) {
			found = c
		}
	}
	return found
}

// lastOutputLine is the last non-empty line of a command's output.
func lastOutputLine(out string) string {
	lines := strings.Split(strings.TrimRight(out, " \t\r\n"), "\n")
	return strings.TrimRight(lines[len(lines)-1], "\r")
}

func (s *scanner) inflight(line chatRolloutLine) {
	p := line.Payload
	switch {
	case line.Type == "event_msg" && p.Type == "task_started":
		s.calls = s.calls[:0]
		clear(s.yielded)
		clear(s.waits)
	case line.Type != "response_item" || p.CallID == "":
	case p.Type == "function_call" || p.Type == "custom_tool_call":
		if s.execShape && p.Type == "custom_tool_call" && p.Name == "exec" && strings.Contains(p.Input, execCall) {
			s.calls = append(s.calls, openCall{p.CallID, transcript.NewInFlightTool("exec_command", map[string]any{"cmd": execCommand(p.Input)})})
			return
		}
		if s.execShape && p.Name == "wait" {
			if id, ok := s.yielded[waitCellID(p.Arguments)]; ok {
				s.waits[p.CallID] = id
				return
			}
		}
		args := map[string]any{}
		if json.Unmarshal([]byte(p.Arguments), &args) != nil {
			args = map[string]any{"command": firstLine(p.Input)}
		}
		s.calls = append(s.calls, openCall{p.CallID, transcript.NewInFlightTool(p.Name, args)})
	case p.Type == "function_call_output" || p.Type == "custom_tool_call_output":
		if s.execShape {
			out := parseCellOutput(p.Output)
			if id, ok := s.waits[p.CallID]; ok {
				if _, ended := out.scriptEnd(); ended {
					s.dropCall(id)
				}
				return
			}
			if cellID, ok := out.cellID(); ok && p.Type == "custom_tool_call_output" {
				s.yielded[cellID] = p.CallID
				return
			}
		}
		s.dropCall(p.CallID)
	}
}
