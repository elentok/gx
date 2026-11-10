package agentfake

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"

	"github.com/elentok/gx/testutil/agentfake/claudesettings"
)

// AskCall is the AskUserQuestion call an ActionAskQuestion runs the hook for.
type AskCall struct {
	ToolUseID string        `json:"tool_use_id"`
	Questions []AskQuestion `json:"questions"`
}

// AskQuestion is one question of an AskCall, in claude's own field names.
type AskQuestion struct {
	Header      string      `json:"header"`
	Question    string      `json:"question"`
	MultiSelect bool        `json:"multiSelect"`
	Options     []AskOption `json:"options"`
}

// AskOption is one choice of an AskQuestion.
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// askQuestion runs the settings' AskUserQuestion hook with the PreToolUse
// payload and holds the session busy until it exits. The hook's output is not
// read: an answer and a release both end the turn.
func (t *tui) askQuestion(call *AskCall) error {
	if call == nil {
		return errors.New("ActionAskQuestion needs Ask")
	}
	command := claudesettings.HookCommand(t.opts.settings, "PreToolUse", "AskUserQuestion")
	if command == "" {
		return errors.New("no PreToolUse hook for AskUserQuestion in --settings")
	}
	payload, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "AskUserQuestion",
		"tool_use_id":     call.ToolUseID,
		"tool_input":      map[string]any{"questions": call.Questions},
	})
	if err != nil {
		return err
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = t.pid.Cwd
	cmd.Env = os.Environ()
	cmd.Stdin = bytes.NewReader(payload)
	if err := cmd.Start(); err != nil {
		return err
	}
	t.askHook = cmd
	// The hook blocks, so it is waited on off the input loop; the turn ends
	// through the control channel like any other control.
	go func() {
		_ = cmd.Wait()
		_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
	}()
	return t.setStatus(StatusBusy, "")
}
