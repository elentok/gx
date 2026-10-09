package nativerunner

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/elentok/gx/agentrunner"
)

// requiredCapabilities are the init capabilities the headless runner relies
// on: Prompt detects a started turn through command_lifecycle. Capabilities
// are checked rather than claude versions, since a version says nothing about
// which protocol features a build ships.
var requiredCapabilities = []string{"msg_lifecycle_v1"}

// event holds the few stream-json fields the headless runner reads. Claude
// adds event types and fields between releases, so anything unknown is
// ignored rather than rejected.
type event struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	UUID      string `json:"uuid"`
	SessionID string `json:"session_id"`
	// State is the session state on session_state_changed and the command
	// state on command_lifecycle.
	State           string `json:"state"`
	CommandUUID     string `json:"command_uuid"`
	Status          string `json:"status"`
	CompactMetadata struct {
		Trigger string `json:"trigger"`
	} `json:"compact_metadata"`
	// RequestID and Request describe a control_request claude sends; only
	// can_use_tool (a permission prompt) is read.
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype  string          `json:"subtype"`
		ToolName string          `json:"tool_name"`
		Input    json.RawMessage `json:"input"`
	} `json:"request"`
	Capabilities  []string   `json:"capabilities"`
	Tasks         []struct{} `json:"tasks"`
	RateLimitInfo struct {
		Status   string `json:"status"`
		ResetsAt int64  `json:"resetsAt"`
	} `json:"rate_limit_info"`
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

// tracker is everything folded from claude's events. Status is derived from
// it, so a turn only counts as finished once nothing else will wake claude.
type tracker struct {
	sessionID string
	turn      int
	working   bool
	blocked   bool
	// permission is the open can_use_tool request. Claude writes it to
	// out.jsonl and nothing records our answer there, so replaying the file
	// after a restart rebuilds it until claude moves on.
	permission *permission
	// turnCmd is set once a gx command has claimed the current turn, so a
	// second queued command starting inside it counts as a new turn.
	turnCmd bool
	// commands maps each gx command uuid to its last lifecycle state.
	commands map[string]string
	bgTasks  int
	resetAt  time.Time
	exited   bool
	// missingCap names the first required capability init did not list.
	missingCap string
	// finished is set by a result and cleared when a turn begins.
	finished bool
	// seen holds event uuids already applied, so replaying out.jsonl after a
	// reattach never counts an event twice.
	seen map[string]bool
}

func (t *tracker) status() agentrunner.Status {
	st := agentrunner.Status{Turn: t.turn, SessionID: t.sessionID}
	switch {
	case t.exited:
		st.State = agentrunner.StateDone
	case t.blocked:
		st.State = agentrunner.StateBlocked
		st.BlockedReason = t.permission.reason()
	case t.working || t.bgTasks > 0 || t.queued():
		st.State = agentrunner.StateWorking
	default:
		st.State = agentrunner.StateIdle
	}
	return st
}

// permission is a tool-use request waiting for an allow or deny.
type permission struct {
	requestID string
	tool      string
	input     json.RawMessage
}

// reason names the tool and its input, so a person can judge the request
// without opening the log. A nil permission is a block claude gave no detail on.
func (p *permission) reason() string {
	if p == nil {
		return "waiting on a prompt"
	}
	return fmt.Sprintf("permission to use %s: %s", p.tool, p.input)
}

// queued reports a gx command claude accepted but has not started; claude
// will start it on its own once the current turn ends.
func (t *tracker) queued() bool {
	for _, state := range t.commands {
		if state == "queued" {
			return true
		}
	}
	return false
}

// apply folds one event into t.
func (t *tracker) apply(e event) {
	if e.UUID != "" {
		if t.seen[e.UUID] {
			return
		}
		if t.seen == nil {
			t.seen = map[string]bool{}
		}
		t.seen[e.UUID] = true
	}
	switch e.Type {
	case "system":
		switch e.Subtype {
		case "init":
			t.sessionID = e.SessionID
			t.missingCap = ""
			for _, c := range requiredCapabilities {
				if !slices.Contains(e.Capabilities, c) {
					t.missingCap = c
					break
				}
			}
		case "session_state_changed":
			switch e.State {
			case "running":
				t.blocked, t.permission = false, nil
				t.beginTurn()
			case "idle":
				t.blocked, t.permission = false, nil
				t.working = false
			case "requires_action":
				t.blocked = true
			}
		case "status":
			if e.Status == "compacting" {
				t.beginTurn()
			}
		case "compact_boundary":
			// An auto compaction happens inside a turn; only the one gx asked
			// for is a turn of its own.
			if e.CompactMetadata.Trigger == "manual" {
				t.working = false
			}
		case "background_tasks_changed":
			t.bgTasks = len(e.Tasks)
		}
	case "command_lifecycle":
		if t.commands == nil {
			t.commands = map[string]string{}
		}
		t.commands[e.CommandUUID] = e.State
		if e.State == "started" {
			if t.working && t.turnCmd {
				t.turn++
			}
			t.beginTurn()
			t.turnCmd = true
		}
	case "control_request":
		if e.Request.Subtype == "can_use_tool" {
			t.permission = &permission{requestID: e.RequestID, tool: e.Request.ToolName, input: e.Request.Input}
			t.blocked = true
		}
	case "control_cancel_request":
		if t.permission != nil && t.permission.requestID == e.RequestID {
			t.permission, t.blocked = nil, false
		}
	case "rate_limit_event":
		t.resetAt = time.Time{}
		if e.RateLimitInfo.Status == "rejected" {
			t.resetAt = time.Unix(e.RateLimitInfo.ResetsAt, 0)
		}
	case "assistant", "stream_event":
		t.beginTurn()
	case "result":
		t.working = false
		t.finished = true
	}
}

// ended reports a turn that ran to its result with no gx command left
// waiting for the next one.
func (t *tracker) ended() bool {
	return t.finished && !t.working && !t.queued()
}

func (t *tracker) beginTurn() {
	t.finished = false
	if !t.working {
		t.working = true
		t.turn++
		t.turnCmd = false
	}
}

func (t *tracker) capabilityErr() error {
	if t.missingCap == "" {
		return nil
	}
	return fmt.Errorf("%w: %s", agentrunner.ErrMissingCapability, t.missingCap)
}

// started reports whether claude started the gx command uuid.
func (t *tracker) started(uuid string) bool {
	state := t.commands[uuid]
	return state == "started" || state == "completed"
}
