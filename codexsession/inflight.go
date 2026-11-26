package codexsession

import "github.com/elentok/gx/transcript"

// ReadInFlightTools lists the function and custom tool calls of the rollout's
// current turn that have no output yet, newest first. The turn starts at the
// last task_started event, so a call an earlier turn left open never shows. A
// missing file has nothing in flight.
//
// With execShape (a rollout of Codex 0.144.1 or later), an open or yielded exec
// cell that runs a command shows as a shell call labelled by the command, its
// wait polls add no entry, a cell that returned a session_id is a background
// process, not in flight, and a command an item_completed line ended is done.
func ReadInFlightTools(path string, execShape bool) ([]transcript.InFlightTool, error) {
	r, err := ReadRollout(path, execShape)
	return r.InFlight, err
}

// ReadChildInFlightTools is ReadInFlightTools over a Codex child's own lines:
// from the first line whose ordinal is at least startOrdinal.
func ReadChildInFlightTools(path string, startOrdinal int, execShape bool) ([]transcript.InFlightTool, error) {
	r, err := readRollout(path, execShape, &startOrdinal)
	return r.InFlight, err
}
