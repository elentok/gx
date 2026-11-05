package agentfake

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/elentok/gx/harness"
)

// TUIEnv, set to "1", makes a test binary that calls TUIClaude from its
// TestMain act as interactive claude on a PTY.
const TUIEnv = "GX_AGENTFAKE_TUI"

// Launch knobs for the TUI fake. They are env vars rather than controls
// because they act before the fake's first status write, before a test could
// send anything.
const (
	// TUITrustEnv, set to "1", shows the trust dialog as claude does: "No,
	// exit" selected first, arrows move the cursor, Enter picks.
	TUITrustEnv = "GX_AGENTFAKE_TUI_TRUST"
	// TUITrustDropKeyEnv, set to "1", drops the first key at the trust
	// dialog, as claude does with one typed before it reads input.
	TUITrustDropKeyEnv = "GX_AGENTFAKE_TUI_TRUST_DROP_KEY"
	// TUINoPidFileEnv, set to "1", never writes the pid status file.
	TUINoPidFileEnv = "GX_AGENTFAKE_TUI_NO_PID_FILE"
	// TUIMouseEnv, set to "1", runs in the alternate screen with SGR mouse
	// reporting on, as claude does, and counts the wheel events it reads on
	// screen. An Esc then only interrupts as part of a sequence.
	TUIMouseEnv = "GX_AGENTFAKE_TUI_MOUSE"
	// TUIDraftEnv, when not empty, seeds the input with that draft and draws
	// the input row as claude does: a styled marker, and a dim placeholder while
	// empty, so a draft check can decide.
	TUIDraftEnv = "GX_AGENTFAKE_TUI_DRAFT"
	// TUIClearLeavesIEnv, set to "1", makes the first clear key leave a lone "i"
	// in the input; the second clears it.
	TUIClearLeavesIEnv = "GX_AGENTFAKE_TUI_CLEAR_LEAVES_I"
	// TUIRegistryDelayEnv, a Go duration, keeps the skill registry empty for
	// that long after start: a skill prompt typed earlier is an unknown command.
	TUIRegistryDelayEnv = "GX_AGENTFAKE_TUI_REGISTRY_DELAY"
	// TUIUnknownSkillEnv, set to "1", makes every skill prompt an unknown
	// command: the pane draws "Unknown command: /<skill>" and the transcript
	// gets a system record and no user record.
	TUIUnknownSkillEnv = "GX_AGENTFAKE_TUI_UNKNOWN_SKILL"
	// TUINoTurnEnv, set to "1", leaves the agent idle after a skill prompt: no
	// turn, no transcript record.
	TUINoTurnEnv = "GX_AGENTFAKE_TUI_NO_TURN"
	// TUILaunchRecordEnv makes a skill prompt's first transcript record the
	// user record claude writes for the command. On by default; "0" turns it off.
	TUILaunchRecordEnv = "GX_AGENTFAKE_TUI_LAUNCH_RECORD"
	// TUIReadyDelayEnv, a Go duration, sets how long after start the fake holds
	// back the ready signal a launch prompt waits for: an input box and a
	// transcript file. Unset, the signal comes at once; TUIReadyOff gives none.
	TUIReadyDelayEnv = "GX_AGENTFAKE_TUI_READY_DELAY"
	// TUIReadyOff is the TUIReadyDelayEnv value for the "signal missing" path.
	TUIReadyOff = "off"
)

// UnknownCommandText is what claude draws, and records, for a skill it doesn't know.
func UnknownCommandText(skill string) string { return "Unknown command: " + skill }

// PermissionPromptPrefix, at the start of a submitted prompt, makes the TUI
// fake ask permission to run the rest of the prompt as a Bash command, with no
// control needed.
const PermissionPromptPrefix = "fakeclaude:permission "

// PermissionCommand is the Bash command a BehaviourPermission worker asks to run.
const PermissionCommand = "rg fixture"

// BashPermissionDialog is the screen rows of a Bash permission prompt for command.
func BashPermissionDialog(command string) []string {
	return []string{
		"────────────────────",
		"Bash command",
		"  " + command,
		"Do you want to proceed?",
		"❯ 1. Yes",
		"  2. No",
		"Esc to cancel",
	}
}

// QuestionMenuDialog is the screen rows of a question menu: a numbered choice
// list that is not an approvable permission card.
func QuestionMenuDialog() []string {
	return []string{
		"────────────────────",
		"Question",
		"  Which option should the fake worker take?",
		"Do you want option A or option B?",
		"❯ 1. Option A",
		"  2. Option B",
		"Esc to cancel",
	}
}

// ActionExit makes the TUI fake exit with Control.Code.
const ActionExit = "exit"

// Pid status file values, as real claude writes them.
const (
	StatusBusy    = "busy"
	StatusIdle    = "idle"
	StatusWaiting = "waiting"
)

// PidFile is the shape of claude's ~/.claude/sessions/<pid>.json, limited to
// the fields gx reads plus the ones that identify the process. Status is
// absent while starting; WaitingFor is absent unless Status is waiting.
type PidFile struct {
	Pid             int    `json:"pid"`
	SessionID       string `json:"sessionId"`
	Cwd             string `json:"cwd"`
	StartedAt       int64  `json:"startedAt"`
	ProcStart       string `json:"procStart"`
	Version         string `json:"version"`
	Kind            string `json:"kind"`
	Entrypoint      string `json:"entrypoint"`
	Status          string `json:"status,omitempty"`
	WaitingFor      string `json:"waitingFor,omitempty"`
	UpdatedAt       int64  `json:"updatedAt"`
	StatusUpdatedAt int64  `json:"statusUpdatedAt,omitempty"`
}

// PidFilePath is where claude running as pid under home writes its status.
func PidFilePath(home string, pid int) string {
	return filepath.Join(home, ".claude", "sessions", strconv.Itoa(pid)+".json")
}

// pidPathFor is PidFilePath, moved under CLAUDE_CONFIG_DIR when it is set, as
// real claude does.
func pidPathFor(home string, pid int) string {
	if dir, err := harness.ClaudeSessionsDirIn(os.Environ()); err == nil {
		return filepath.Join(dir, strconv.Itoa(pid)+".json")
	}
	return PidFilePath(home, pid)
}

// TUIControlPath is the side-channel FIFO of the TUI fake running sessionID
// under home. Controls never travel on the PTY, so typed input stays exactly
// what the runner sent.
func TUIControlPath(home, sessionID string) string {
	return filepath.Join(home, ".claude", "agentfake", sessionID+".ctl")
}

// TUIInputPath logs every key the TUI fake running sessionID under home read
// off its terminal.
func TUIInputPath(home, sessionID string) string {
	return filepath.Join(home, ".claude", "agentfake", sessionID+".in")
}

// TUIAckPath logs one byte per control the TUI fake running sessionID under
// home has handled.
func TUIAckPath(home, sessionID string) string {
	return filepath.Join(home, ".claude", "agentfake", sessionID+".ack")
}

// TUIArgsPath holds the argv (a JSON array) the TUI fake running sessionID
// under home was started with.
func TUIArgsPath(home, sessionID string) string {
	return filepath.Join(home, ".claude", "agentfake", sessionID+".args")
}

// ReadTUIArgs is the argv the TUI fake running sessionID under home was
// started with.
func ReadTUIArgs(home, sessionID string) ([]string, error) {
	data, err := os.ReadFile(TUIArgsPath(home, sessionID))
	if err != nil {
		return nil, err
	}
	var args []string
	return args, json.Unmarshal(data, &args)
}

// ReportedModel is the model the TUI fake reports on its replies when it was
// started without --model; StaleModel makes it exit at once, as claude does
// for a model it no longer knows.
const (
	ReportedModel = "claude-fake-1"
	StaleModel    = "stale-model"
)

// SendTUIControl writes c to the side channel of the TUI fake running
// sessionID under home, waiting for the fake to create it and then to handle
// c. The control FIFO and the PTY's keys reach the fake on separate paths, so
// a key sent before the ack can beat the control. ActionExit gets no ack.
func SendTUIControl(home, sessionID string, c Control) error {
	ackPath := TUIAckPath(home, sessionID)
	var handled int64
	if st, err := os.Stat(ackPath); err == nil {
		handled = st.Size()
	}
	ctlPath := TUIControlPath(home, sessionID)
	if err := sendControlWhenReady(func() (string, bool) {
		_, err := os.Stat(ctlPath)
		return ctlPath, err == nil
	}, c); err != nil {
		return err
	}
	if c.Action == ActionExit {
		return nil
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		if st, err := os.Stat(ackPath); err == nil && st.Size() > handled {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("agentfake: control %s not handled", c.Action)
		}
	}
}

// sendControlWhenReady writes c to the side-channel FIFO that ready reports,
// waiting until ready says the fake has it open. A FIFO that merely exists has
// no reader yet, and a write to it fails with ENXIO.
func sendControlWhenReady(ready func() (path string, ok bool), c Control) error {
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		path, ok := ready()
		if ok {
			return SendControl(path, c)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("agentfake: %s never opened", path)
		}
	}
}
