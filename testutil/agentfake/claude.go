package agentfake

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/elentok/gx/claudedoctor"
)

// ClaudeEnv, set to "1", makes a test binary that calls Claude from its
// TestMain act as `claude -p` speaking stream-json.
const ClaudeEnv = "GX_AGENTFAKE_CLAUDE"

// Actions a Control message asks the fake claude to take.
const (
	// ActionFinish ends the current turn (or /compact) with a result.
	ActionFinish = "finish"
	// ActionBlock waits on a permission prompt gx did not send.
	ActionBlock = "block"
	// ActionRateLimit reports a rejected rate limit resetting at ResetsAt.
	ActionRateLimit = "ratelimit"
	// ActionStall leaves every later prompt unstarted.
	ActionStall = "stall"
	// ActionBackground starts (Running) or ends a background task.
	ActionBackground = "background"
)

// Control stands in for what real claude does on its own. It travels on the
// agent's stdin like a prompt, so the fake handles it in order with gx's
// messages and the test never has to wait for the fake to catch up.
type Control struct {
	Action   string `json:"action"`
	ResetsAt int64  `json:"resets_at,omitempty"`
	Running  bool   `json:"running,omitempty"`
}

const controlType = "agentfake_control"

// SendControl writes c to the agent's stdin FIFO. One write of a short line
// is atomic, so it never interleaves with gx's own writes.
func SendControl(stdin string, c Control) error {
	f, err := os.OpenFile(stdin, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(struct {
		Type string `json:"type"`
		Control
	}{controlType, c})
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// Claude runs the fake claude over stdin and stdout and exits.
func Claude() {
	sessionID := os.Args[slices.Index(os.Args, "--session-id")+1]
	init, err := initEvent(sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := runClaude(os.Stdin, os.Stdout, init); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// initEvent is the doctor's recorded init event, re-keyed to sessionID.
func initEvent(sessionID string) (string, error) {
	src, err := claudedoctor.FixtureSource()
	if err != nil {
		return "", err
	}
	events, err := src.Session("init")
	if err != nil {
		return "", err
	}
	var e map[string]any
	if err := json.Unmarshal(events[0], &e); err != nil {
		return "", err
	}
	e["session_id"] = sessionID
	data, err := json.Marshal(e)
	return string(data), err
}

type message struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	RequestID string `json:"request_id"`
	Message   struct {
		Content string `json:"content"`
	} `json:"message"`
	Control
}

func runClaude(in io.Reader, out io.Writer, init string) error {
	emit := func(lines ...string) {
		for _, l := range lines {
			fmt.Fprintln(out, l)
		}
	}
	lifecycle := func(uuid, state string) string {
		return fmt.Sprintf(`{"type":"command_lifecycle","command_uuid":%q,"state":%q}`, uuid, state)
	}

	var (
		initSent, stalled, compacting bool
		// cmd is the gx command whose turn is running.
		cmd string
	)
	endTurn := func(subtype string) {
		emit(fmt.Sprintf(`{"type":"result","subtype":%q}`, subtype))
		if cmd != "" {
			emit(lifecycle(cmd, "completed"))
			cmd = ""
		}
	}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var msg message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return fmt.Errorf("bad stdin line %q: %w", scanner.Text(), err)
		}
		switch msg.Type {
		case "user":
			if stalled {
				continue
			}
			if msg.Message.Content == "/compact" {
				// Claude runs /compact without a command lifecycle.
				emit(`{"type":"system","subtype":"status","status":"compacting"}`)
				compacting = true
				continue
			}
			cmd = msg.UUID
			emit(lifecycle(cmd, "queued"), lifecycle(cmd, "started"))
			if !initSent {
				emit(init)
				initSent = true
			}
			emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`)
		case "control_request":
			emit(fmt.Sprintf(`{"type":"control_response","response":{"subtype":"success","request_id":%q}}`, msg.RequestID))
			endTurn("error_during_execution")
		case controlType:
			switch msg.Action {
			case ActionFinish:
				if compacting {
					emit(`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"manual"}}`)
					compacting = false
				}
				endTurn("success")
			case ActionBlock:
				emit(`{"type":"system","subtype":"session_state_changed","state":"requires_action"}`)
			case ActionRateLimit:
				emit(fmt.Sprintf(`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":%d}}`, msg.ResetsAt))
			case ActionStall:
				stalled = true
			case ActionBackground:
				tasks := `[]`
				if msg.Running {
					tasks = `[{"task_id":"b1"}]`
				}
				emit(fmt.Sprintf(`{"type":"system","subtype":"background_tasks_changed","tasks":%s}`, tasks))
			default:
				return fmt.Errorf("unknown control action %q", msg.Action)
			}
		}
	}
	return scanner.Err()
}
