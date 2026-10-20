package agentfake

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

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
	// ActionStall leaves every later prompt unstarted, and on the TUI fake a
	// dialog open on Esc.
	ActionStall = "stall"
	// ActionBackground starts (Running) or ends a background task.
	ActionBackground = "background"
	// ActionFast makes the TUI fake's next turn start and end at once, so a
	// poll of its pid file sees no busy edge; only the OSC reports show it.
	ActionFast = "fast"
	// ActionDialog makes the TUI fake open a dialog on the next "/" command,
	// as /help does.
	ActionDialog = "dialog"
)

// Control stands in for what real claude does on its own. It travels on the
// agent's stdin like a prompt, so the fake handles it in order with gx's
// messages and the test never has to wait for the fake to catch up.
type Control struct {
	Action   string `json:"action"`
	ResetsAt int64  `json:"resets_at,omitempty"`
	Running  bool   `json:"running,omitempty"`
	// WaitingFor is the dialog an ActionBlock shows on the TUI fake.
	WaitingFor string `json:"waiting_for,omitempty"`
	// Code is the ActionExit exit code of the TUI fake and the fake program.
	Code int `json:"code,omitempty"`
	// State, Kind and Message are the OSC 7501 report an ActionReport makes
	// the fake program write.
	State   string `json:"state,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
	// Title is the OSC 0 title an ActionTitle makes the fake program set.
	Title string `json:"title,omitempty"`
	// Output is the raw terminal output an ActionWrite makes the fake program
	// write, such as a mode switch or a terminal query.
	Output string `json:"output,omitempty"`
	// ConversationID is the id an ActionConversation makes the fake program
	// report.
	ConversationID string `json:"conversation_id,omitempty"`
}

const controlType = "agentfake_control"

// fifoOpenWait bounds how long SendControl waits for the agent to open its end
// of the FIFO.
const fifoOpenWait = 5 * time.Second

// openFIFOWriter opens the FIFO at path for writing without blocking. A plain
// O_WRONLY open waits forever when the agent has died, so a test that races an
// agent's exit would hang until the package timeout; ENXIO means no reader yet.
func openFIFOWriter(path string) (*os.File, error) {
	for deadline := time.Now().Add(fifoOpenWait); ; time.Sleep(10 * time.Millisecond) {
		fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			return os.NewFile(uintptr(fd), path), nil
		}
		if err != syscall.ENXIO || time.Now().After(deadline) {
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		}
	}
}

// SendControl writes c to the agent's stdin FIFO. One write of a short line
// is atomic, so it never interleaves with gx's own writes.
func SendControl(stdin string, c Control) error {
	f, err := openFIFOWriter(stdin)
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
	go exitWithParent()
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

// exitWithParent ends the fake once the process that started it is gone. It
// talks over pipes, so unlike the pty-hosted fakes nothing hangs up on it when
// a test binary is killed before its cleanup, and it would otherwise wait on
// its stdin FIFO forever.
func exitWithParent() {
	parent := os.Getppid()
	for range time.Tick(200 * time.Millisecond) {
		if os.Getppid() != parent {
			os.Exit(1)
		}
	}
}

// initEvent is the doctor's recorded init event, re-keyed to sessionID.
func initEvent(sessionID string) (string, error) {
	src, err := claudedoctor.FixtureSource()
	if err != nil {
		return "", err
	}
	rec, err := src.Session("init")
	if err != nil {
		return "", err
	}
	var e map[string]any
	if err := json.Unmarshal(rec.Events[0], &e); err != nil {
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
	// Response is gx's answer to a permission request.
	Response struct {
		RequestID string `json:"request_id"`
		Response  struct {
			Behavior string `json:"behavior"`
			Message  string `json:"message"`
		} `json:"response"`
	} `json:"response"`
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
		// queue holds the gx commands claude accepted behind the running turn.
		queue []string
		// dialog makes the next "/" command open a dialog, as /help does.
		dialog bool
	)
	sendInit := func() {
		if !initSent {
			emit(init)
			initSent = true
		}
	}
	startCmd := func(uuid string) {
		cmd = uuid
		emit(lifecycle(cmd, "started"))
		sendInit()
		emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`)
	}
	endTurn := func(subtype string) {
		emit(fmt.Sprintf(`{"type":"result","subtype":%q}`, subtype))
		if cmd != "" {
			emit(lifecycle(cmd, "completed"))
			cmd = ""
		}
		if len(queue) > 0 {
			next := queue[0]
			queue = queue[1:]
			startCmd(next)
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
			// The agent's own record of what it received, for tests to read back.
			text, _ := json.Marshal(msg.Message.Content)
			emit(fmt.Sprintf(`{"type":"agentfake_received","text":%s}`, text))
			if stalled {
				continue
			}
			switch content := msg.Message.Content; {
			case content == "/compact":
				// Claude runs /compact without a command lifecycle.
				emit(`{"type":"system","subtype":"status","status":"compacting"}`)
				compacting = true
			case strings.HasPrefix(content, "/"):
				// Most "/" commands run with no turn and no lifecycle event.
				sendInit()
				switch {
				case dialog:
					dialog = false
					emit(`{"type":"system","subtype":"session_state_changed","state":"requires_action"}`)
				case content == "/clear":
					emit(lifecycle(msg.UUID, "queued"), lifecycle(msg.UUID, "started"))
					cmd = msg.UUID
					endTurn("success")
				}
			case cmd != "":
				emit(lifecycle(msg.UUID, "queued"))
				queue = append(queue, msg.UUID)
			default:
				emit(lifecycle(msg.UUID, "queued"))
				startCmd(msg.UUID)
			}
		case "control_request":
			emit(fmt.Sprintf(`{"type":"control_response","response":{"subtype":"success","request_id":%q}}`, msg.RequestID))
			endTurn("error_during_execution")
		case "control_response":
			// The fake echoes the decision so a test can read what claude was told.
			answer := msg.Response.Response
			emit(
				`{"type":"system","subtype":"session_state_changed","state":"running"}`,
				fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":%q}]}}`,
					strings.TrimSpace("permission "+answer.Behavior+" "+answer.Message)),
			)
		case controlType:
			switch msg.Action {
			case ActionFinish:
				if compacting {
					emit(`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"manual"}}`)
					compacting = false
				}
				endTurn("success")
			case ActionBlock:
				emit(
					`{"type":"control_request","request_id":"perm-1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls"}}}`,
					`{"type":"system","subtype":"session_state_changed","state":"requires_action"}`,
				)
			case ActionRateLimit:
				emit(fmt.Sprintf(`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":%d}}`, msg.ResetsAt))
			case ActionStall:
				stalled = true
			case ActionDialog:
				dialog = true
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
