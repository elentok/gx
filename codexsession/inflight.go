package codexsession

import (
	"encoding/json"
	"os"
	"slices"

	"github.com/elentok/gx/transcript"
)

// ReadInFlightTools lists the function and custom tool calls of the rollout's
// current turn that have no output yet, newest first. The turn starts at the
// last task_started event, so a call an earlier turn left open never shows. A
// missing file has nothing in flight.
func ReadInFlightTools(path string) ([]transcript.InFlightTool, error) {
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
	err = transcript.ForEachLine(f, func(raw []byte) {
		var line chatRolloutLine
		if json.Unmarshal(raw, &line) != nil {
			return
		}
		p := line.Payload
		switch {
		case line.Type == "event_msg" && p.Type == "task_started":
			calls = calls[:0]
		case line.Type != "response_item" || p.CallID == "":
		case p.Type == "function_call" || p.Type == "custom_tool_call":
			args := map[string]any{}
			if json.Unmarshal([]byte(p.Arguments), &args) != nil {
				args = map[string]any{"command": firstLine(p.Input)}
			}
			calls = append(calls, open{p.CallID, transcript.NewInFlightTool(p.Name, args)})
		case p.Type == "function_call_output" || p.Type == "custom_tool_call_output":
			calls = slices.DeleteFunc(calls, func(c open) bool { return c.id == p.CallID })
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
