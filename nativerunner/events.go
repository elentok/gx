package nativerunner

import (
	"encoding/json"

	"github.com/elentok/gx/agentrunner"
)

// event holds the few stream-json fields the headless runner reads. Claude
// adds event types and fields between releases, so anything unknown is
// ignored rather than rejected.
type event struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

// parseEvent reports false for a line that isn't a JSON object with a type,
// e.g. a stray log line claude printed to stdout.
func parseEvent(line []byte) (event, bool) {
	var e event
	if json.Unmarshal(line, &e) != nil || e.Type == "" {
		return event{}, false
	}
	return e, true
}

// apply folds one event into st.
func apply(st *agentrunner.Status, e event) {
	switch e.Type {
	case "system":
		switch e.Subtype {
		case "init":
			st.SessionID = e.SessionID
		case "session_state_changed":
			switch e.State {
			case "running":
				beginTurn(st)
			case "idle":
				st.State = agentrunner.StateIdle
			case "requires_action":
				st.State = agentrunner.StateBlocked
			}
		}
	case "assistant", "user", "stream_event":
		beginTurn(st)
	case "result":
		st.State = agentrunner.StateIdle
	}
}

func beginTurn(st *agentrunner.Status) {
	if st.State != agentrunner.StateWorking {
		st.State = agentrunner.StateWorking
		st.Turn++
	}
}
