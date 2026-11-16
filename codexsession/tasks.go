package codexsession

import (
	"encoding/json"
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

// ReadTasks returns one record per exec cell that runs a command and per
// spawn_agent call, oldest first. It reads the whole rollout: a command outlives
// its turn. A row with no matched end keeps a nil End; the assembler's no-end
// rules decide the rest. Ends match exactly, by call id, cell id, session id or
// thread id, except an item_completed line, which has no call id and matches by
// process id, then by command text and time.
func ReadTasks(path string) ([]transcript.TaskRecord, error) {
	r, err := ReadRollout(path, true)
	return r.Records, err
}
