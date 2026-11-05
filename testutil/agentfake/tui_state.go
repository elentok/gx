package agentfake

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

const EscKey = 0x1b

const (
	CtrlC = 0x03
	ctrlU = 0x15
)

// interrupt is Esc: it denies a dialog and keeps the session. On a running
// turn it does nothing: only Ctrl+C cancels that. A stalled fake leaves a
// dialog open.
func (t *tui) interrupt() error {
	t.typed.Reset()
	if t.pid.Status != StatusWaiting || t.stalled {
		return nil
	}
	return t.setStatus(StatusIdle, "")
}

// cancel is Ctrl+C: it ends a running turn and restores the cancelled prompt
// into the input line. On an idle prompt it clears the whole input, as real
// claude does. A stalled fake ignores it.
func (t *tui) cancel() error {
	if t.stalled {
		return nil
	}
	if t.pid.Status == StatusIdle {
		t.typed.Reset()
		t.draw()
		return nil
	}
	if t.pid.Status != StatusBusy {
		return nil
	}
	t.typed.Reset()
	t.typed.WriteString(t.prompt)
	if err := t.setStatus(StatusIdle, ""); err != nil {
		return err
	}
	t.draw()
	return nil
}

func (t *tui) handleControl(c Control) error {
	switch c.Action {
	case ActionFinish:
		if err := t.setStatus(StatusIdle, ""); err != nil {
			return err
		}
		// Off the input loop: claude keeps reading keys while its statusLine
		// command runs, and a slow one must not look like a missed prompt.
		go t.runStatusLine()
		return nil
	case ActionBlock:
		waitingFor := c.WaitingFor
		if waitingFor == "" {
			waitingFor = "permission prompt"
		}
		t.dialog = c.Dialog
		return t.setStatus(StatusWaiting, waitingFor)
	case ActionStall:
		t.stalled = true
		return nil
	case ActionBackground:
		return t.writeBackgroundMarker(c.Running)
	case ActionFast:
		t.fast = true
		return nil
	case ActionDialog:
		t.dialogNext = true
		return nil
	case ActionBusy:
		t.busyNext = true
		return nil
	case ActionCommandOutput:
		t.commandOutput = &c.Message
		return nil
	case ActionIgnoreAllow:
		t.ignoreAllow = true
		return nil
	case ActionReport:
		writeReport(t.out, c.State, c.Kind, c.Message)
		return nil
	case ActionConversation:
		t.pid.SessionID = c.ConversationID
		return t.writePid()
	case ActionAPIError:
		return t.apiError(c)
	case ActionModel:
		t.opts.model = c.Model
		return nil
	case ActionVimToggle:
		t.vim = true
		t.draw()
		return writeVimSetting(claudeSettingsPath())
	default:
		return fmt.Errorf("unknown control action %q", c.Action)
	}
}

func (t *tui) setStatus(status, waitingFor string) error {
	t.pid.Status = status
	t.pid.WaitingFor = waitingFor
	t.pid.StatusUpdatedAt = time.Now().UnixMilli()
	t.draw()
	return t.writePid()
}

// writePid replaces the pid file in one rename, so a reader never sees half
// of it.
func (t *tui) writePid() error {
	if t.opts.noPidFile {
		return nil
	}
	t.pid.UpdatedAt = time.Now().UnixMilli()
	data, err := json.Marshal(t.pid)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(t.pidPath), 0o755); err != nil {
		return err
	}
	tmp := t.pidPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, t.pidPath)
}

// exit removes the pid file on a clean exit, as claude does; a crash leaves
// it behind.
func (t *tui) exit(code int) error {
	if code != 0 || t.opts.noPidFile {
		return nil
	}
	if err := os.Remove(t.pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// writeBackgroundMarker appends the transcript line claude writes when a
// background task starts, or its completion notification.
func (t *tui) writeBackgroundMarker(running bool) error {
	const taskID, toolUseID = "b1", "toolu_b1"
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	line := `{"isSidechain":false,"type":"user","timestamp":"` + ts + `","message":{"content":[{"tool_use_id":"` + toolUseID + `","type":"tool_result","content":"...","is_error":false}]},"toolUseResult":{"stdout":"","backgroundTaskId":"` + taskID + `"}}`
	if !running {
		content := `<task-notification>\n<task-id>` + taskID + `</task-id>\n<tool-use-id>` + toolUseID + `</tool-use-id>\n<status>completed</status>\n</task-notification>`
		line = `{"isSidechain":false,"type":"user","timestamp":"` + ts + `","origin":{"kind":"task-notification"},"message":{"content":"` + content + `"}}`
	}
	if err := os.MkdirAll(filepath.Dir(t.transcript), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(t.transcript, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

// colored wraps s in red when the terminal supports color. Like claude
// (supports-color), it decides from TERM and COLORTERM alone, so a session
// without them is colorless.
func colored(s string) string {
	term := os.Getenv("TERM")
	if os.Getenv("COLORTERM") == "" && (term == "" || term == "dumb") {
		return s
	}
	return "\x1b[31m" + s + "\x1b[0m"
}

// draw clears the terminal and redraws the screen for the current state.
func (t *tui) draw() {
	title := idleTitle
	var rows []string
	switch {
	case t.opts.trust:
		no, yes := "❯ No, exit", "  Yes, I trust this folder"
		if t.trustYes {
			no, yes = "  No, exit", "❯ Yes, I trust this folder"
		}
		rows = []string{"Quick safety check: Is this a project you created or one you trust?", "", no, yes}
	case t.pid.Status == StatusWaiting:
		rows = []string{
			"> " + t.prompt,
			"Waiting for " + t.pid.WaitingFor,
			"Do you want to proceed?",
			"❯ 1. Yes",
			"  2. No",
		}
		if t.dialog != nil {
			rows = t.dialog
		}
	case t.pid.Status == StatusBusy:
		title = workingTitle
		rows = []string{"> " + t.prompt, "✻ Working…"}
	case t.opts.draft != "":
		input := promptMarker + t.typed.String()
		if t.typed.Len() == 0 {
			input = promptMarker + "\x1b[2m" + ProgramPlaceholder + "\x1b[0m"
		}
		rows = []string{colored("✻ Welcome to Claude Code"), "", input}
	case t.opts.readySignal && t.typed.Len() == 0 && t.ready():
		// The input box claudeui.InputBox recognises; typed text echoes as "> ".
		const rule = "────────────────────"
		rows = []string{colored("✻ Welcome to Claude Code"), "", rule, "❯ ", rule}
	case t.opts.readySignal && t.typed.Len() == 0:
		rows = []string{colored("✻ Welcome to Claude Code"), "", "Loading…"}
	default:
		rows = []string{colored("✻ Welcome to Claude Code"), "", "> " + t.typed.String()}
	}
	if t.notice != "" && t.pid.Status == StatusIdle {
		rows = append(rows, t.notice)
	}
	if t.vim && t.pid.Status == StatusIdle {
		rows = append(rows, "-- INSERT --")
	}
	if t.opts.mouse {
		rows = append(rows, fmt.Sprintf("wheel up %d down %d", t.wheelUp, t.wheelDown))
	}
	if cols, _, err := term.GetSize(os.Stdout.Fd()); err == nil && cols > 0 {
		// Claude lays its text out for the width it has and redraws when it changes.
		wrapped := make([]string, len(rows))
		for i, row := range rows {
			wrapped[i] = strings.ReplaceAll(ansi.Wordwrap(row, cols, ""), "\n", "\r\n")
		}
		rows = wrapped
	}
	fmt.Fprint(t.out, "\x1b[2J\x1b[H"+title+strings.Join(rows, "\r\n"))
}
