package codexsession

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/elentok/gx/transcript"
)

const (
	execCall       = "tools.exec_command("
	writeStdinCall = "tools.write_stdin("
)

var (
	// cmdRE matches both {cmd:"x"} and {"cmd":"x"} and captures the string literal.
	cmdRE     = regexp.MustCompile(`(?:"cmd"|\bcmd)\s*:\s*("(?:[^"\\]|\\.)*")`)
	sessionRE = regexp.MustCompile(`(?:"session_id"|\bsession_id)\s*:\s*(\d+)`)
)

// execCommand is the command an exec cell's script runs: the cmd literal, or
// the script's first line when the script builds the command in JS.
func execCommand(input string) string {
	if m := cmdRE.FindStringSubmatch(input); m != nil {
		var s string
		if json.Unmarshal([]byte(m[1]), &s) == nil && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return firstLine(input)
}

// cellOutput is what an exec cell, or a wait poll, printed. head is the first
// text; inner is the first later text that parses as a JSON object (the
// script's own result).
type cellOutput struct {
	head  string
	inner map[string]json.RawMessage
}

// parseCellOutput reads a custom_tool_call_output (a JSON string holding an
// array of texts) or a function_call_output (a plain string).
func parseCellOutput(raw json.RawMessage) cellOutput {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return cellOutput{}
	}
	var texts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal([]byte(s), &texts) != nil || len(texts) == 0 {
		return cellOutput{head: strings.TrimSpace(s)}
	}
	out := cellOutput{head: strings.TrimSpace(texts[0].Text)}
	for _, t := range texts[1:] {
		var m map[string]json.RawMessage
		if json.Unmarshal([]byte(t.Text), &m) == nil && m != nil {
			out.inner = m
			break
		}
	}
	return out
}

// cellID is N of a yielded cell's "Script running with cell ID N".
func (o cellOutput) cellID() (string, bool) {
	id, ok := strings.CutPrefix(o.head, "Script running with cell ID ")
	id = strings.TrimSpace(id)
	return id, ok && id != ""
}

// scriptEnd maps a head to the script's end, ok false while it has none.
func (o cellOutput) scriptEnd() (outcome string, ok bool) {
	switch {
	case strings.HasPrefix(o.head, "Script completed"):
		return "", true
	case strings.HasPrefix(o.head, "Script failed"):
		return transcript.TaskOutcomeFailure, true
	case strings.HasPrefix(o.head, "Script terminated"):
		return transcript.TaskOutcomeKilled, true
	}
	return "", false
}

func (o cellOutput) sessionID() (string, bool) {
	raw, ok := o.inner["session_id"]
	if !ok {
		return "", false
	}
	var n json.Number
	if json.Unmarshal(raw, &n) != nil {
		return "", false
	}
	return n.String(), true
}

func (o cellOutput) exitCode() *int {
	var n int
	if raw, ok := o.inner["exit_code"]; !ok || json.Unmarshal(raw, &n) != nil {
		return nil
	}
	return &n
}

// waitCellID is the cell a wait call polls.
func waitCellID(arguments string) string {
	var a struct {
		CellID json.RawMessage `json:"cell_id"`
	}
	if json.Unmarshal([]byte(arguments), &a) != nil {
		return ""
	}
	return strings.Trim(string(a.CellID), `"`)
}

func parseRolloutTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// ReadTasks returns one shell record per exec cell that runs a command, oldest
// first. It reads the whole rollout: a command outlives its turn. A cell with no
// matched end keeps a nil End; the assembler's no-end rules decide the rest.
// Ends match exactly, by call id, cell id or session id.
func ReadTasks(path string) ([]transcript.TaskRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	type cell struct {
		rec      *transcript.TaskRecord
		commands int
	}
	var cells []*cell
	byCall := map[string]*cell{}
	byCellID := map[string]*cell{}
	byWait := map[string]*cell{}
	bySession := map[string]*cell{}
	pollSession := map[string]string{} // call id of a write_stdin cell -> its session id

	end := func(c *cell, at time.Time, outcome string, code *int) {
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
	}

	err = transcript.ForEachLine(f, func(raw []byte) {
		var line chatRolloutLine
		if json.Unmarshal(raw, &line) != nil || line.Type != "response_item" {
			return
		}
		p := line.Payload
		ts, ok := parseRolloutTime(line.Timestamp)
		if !ok {
			return
		}
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
				c := &cell{rec: rec, commands: strings.Count(p.Input, execCall)}
				cells = append(cells, c)
				byCall[p.CallID] = c
			} else if strings.Count(p.Input, writeStdinCall) == 1 {
				if m := sessionRE.FindStringSubmatch(p.Input); m != nil {
					pollSession[p.CallID] = m[1]
				}
			}
		case "custom_tool_call_output":
			out := parseCellOutput(p.Output)
			if c := byCall[p.CallID]; c != nil {
				if id, ok := out.cellID(); ok {
					byCellID[id] = c
					return
				}
				outcome, ended := out.scriptEnd()
				if !ended {
					return
				}
				code := out.exitCode()
				if sid, ok := out.sessionID(); ok && code == nil {
					bySession[sid] = c
					return
				}
				if c.commands != 1 {
					code = nil
				}
				end(c, ts, outcome, code)
			} else if sid, ok := pollSession[p.CallID]; ok {
				_, stillAlive := out.sessionID()
				if code := out.exitCode(); code != nil && !stillAlive && bySession[sid] != nil {
					end(bySession[sid], ts, "", code)
				}
			}
		case "function_call":
			if p.Name == "wait" {
				if c := byCellID[waitCellID(p.Arguments)]; c != nil {
					byWait[p.CallID] = c
				}
			}
		case "function_call_output":
			if c := byWait[p.CallID]; c != nil {
				if outcome, ended := parseCellOutput(p.Output).scriptEnd(); ended {
					end(c, ts, outcome, nil)
				}
			}
		}
	})
	if err != nil {
		return nil, err
	}
	out := make([]transcript.TaskRecord, len(cells))
	for i, c := range cells {
		out[i] = *c.rec
	}
	return out, nil
}
