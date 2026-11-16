package codexsession

import (
	"encoding/json"
	"os"
	"slices"
	"strings"

	"github.com/elentok/gx/transcript"
)

// ReadInFlightTools lists the function and custom tool calls of the rollout's
// current turn that have no output yet, newest first. The turn starts at the
// last task_started event, so a call an earlier turn left open never shows. A
// missing file has nothing in flight.
//
// With execShape (a rollout of Codex 0.144.1 or later), an open or yielded exec
// cell that runs a command shows as a shell call labelled by the command, its
// wait polls add no entry, and a cell that returned a session_id is a
// background process, not in flight.
func ReadInFlightTools(path string, execShape bool) ([]transcript.InFlightTool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	type open struct {
		id   string
		tool transcript.InFlightTool
	}
	var calls []open
	remove := func(id string) {
		calls = slices.DeleteFunc(calls, func(c open) bool { return c.id == id })
	}
	yielded := map[string]string{} // cell id -> call id of the yielded exec cell
	waits := map[string]string{}   // wait call id -> call id of the cell it polls
	err = transcript.ForEachLine(f, func(raw []byte) {
		var line chatRolloutLine
		if json.Unmarshal(raw, &line) != nil {
			return
		}
		p := line.Payload
		switch {
		case line.Type == "event_msg" && p.Type == "task_started":
			calls = calls[:0]
			clear(yielded)
			clear(waits)
		case line.Type != "response_item" || p.CallID == "":
		case p.Type == "function_call" || p.Type == "custom_tool_call":
			if execShape && p.Type == "custom_tool_call" && p.Name == "exec" && strings.Contains(p.Input, execCall) {
				calls = append(calls, open{p.CallID, transcript.NewInFlightTool("exec_command", map[string]any{"cmd": execCommand(p.Input)})})
				return
			}
			if execShape && p.Name == "wait" {
				if id, ok := yielded[waitCellID(p.Arguments)]; ok {
					waits[p.CallID] = id
					return
				}
			}
			args := map[string]any{}
			if json.Unmarshal([]byte(p.Arguments), &args) != nil {
				args = map[string]any{"command": firstLine(p.Input)}
			}
			calls = append(calls, open{p.CallID, transcript.NewInFlightTool(p.Name, args)})
		case p.Type == "function_call_output" || p.Type == "custom_tool_call_output":
			if execShape {
				out := parseCellOutput(p.Output)
				if id, ok := waits[p.CallID]; ok {
					if _, ended := out.scriptEnd(); ended {
						remove(id)
					}
					return
				}
				if cellID, ok := out.cellID(); ok && p.Type == "custom_tool_call_output" {
					yielded[cellID] = p.CallID
					return
				}
			}
			remove(p.CallID)
		}
	})
	if err != nil {
		return nil, err
	}
	out := make([]transcript.InFlightTool, 0, len(calls))
	for _, c := range slices.Backward(calls) {
		out = append(out, c.tool)
	}
	return out, nil
}
