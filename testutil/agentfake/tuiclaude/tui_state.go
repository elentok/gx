package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/elentok/gx/backgroundtask"
	"github.com/elentok/gx/testutil/agentfake/claudesettings"
)

// interrupt is Esc: it denies a dialog and keeps the session. On a running
// turn it does nothing: only Ctrl+C cancels that. A stalled fake leaves a
// dialog open.
func (t *tui) interrupt() error {
	if t.askHook == nil && len(t.menuRows()) > 0 {
		// Esc on an open suggestion menu only closes it.
		t.menuClosed = true
		t.draw()
		return nil
	}
	t.typed.Reset()
	if t.askHook != nil {
		// Esc on a held dialog ends the hook, as claude does.
		_ = t.askHook.Process.Signal(syscall.SIGTERM)
		return nil
	}
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
	t.compact = nil
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
		t.askHook = nil
		if err := t.finishCompact(); err != nil {
			return err
		}
		if err := t.setStatus(StatusIdle, ""); err != nil {
			return err
		}
		// Off the input loop: claude keeps reading keys while its statusLine
		// command runs, and a slow one must not look like a missed prompt.
		go t.runStatusLine()
		return nil
	case ActionShell:
		return t.setStatus(StatusShell, "")
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
	case ActionToolUse:
		return t.startToolCalls(c.ToolCalls)
	case ActionAskQuestion:
		return t.askQuestion(c.Ask)
	case ActionAskScreen:
		return t.startAskScreen(c.AskScreen)
	case ActionAskEscTimeout:
		return t.askEscTimeout()
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
		WriteReport(t.out, c.State, c.Kind, c.Message)
		return nil
	case ActionConversation:
		t.pid.SessionID = c.ConversationID
		return t.writePid()
	case ActionAPIError:
		return t.apiError(c)
	case ActionDescendants:
		return t.descendants.Start(c.Count, c.Depth)
	case ActionStopDescendants:
		t.descendants.Stop()
		return nil
	case ActionSwallowEnter:
		t.swallow++
		return nil
	case ActionUndecided:
		t.undecidedNext = true
		return nil
	case ActionUnknownSkill:
		t.opts.unknownSkill = true
		return nil
	case ActionModel:
		t.opts.model = c.Model
		return nil
	case ActionUsage:
		return t.appendUsage(c.Tokens)
	case ActionVimToggle:
		t.vim = true
		t.draw()
		return WriteVimSetting(claudesettings.Path())
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
	t.descendants.Stop()
	if code != 0 || t.opts.noPidFile {
		return nil
	}
	if err := os.Remove(t.pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// writeBackgroundMarker appends the transcript line claude writes when a
// background task starts, or its completion notification followed by the
// assistant turn end that the probe waits for.
func (t *tui) writeBackgroundMarker(running bool) error {
	const taskID, toolUseID = "b1", "toolu_b1"
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	line := `{"isSidechain":false,"type":"user","timestamp":"` + ts + `","message":{"content":[{"tool_use_id":"` + toolUseID + `","type":"tool_result","content":"...","is_error":false}]},"toolUseResult":{"stdout":"","backgroundTaskId":"` + taskID + `"}}`
	if !running {
		content := `<task-notification>\n<task-id>` + taskID + `</task-id>\n<tool-use-id>` + toolUseID + `</tool-use-id>\n<status>completed</status>\n</task-notification>`
		line = `{"isSidechain":false,"type":"user","timestamp":"` + ts + `","origin":{"kind":"task-notification"},"message":{"content":"` + content + `"}}` +
			"\n" + `{"isSidechain":false,"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","stop_reason":"end_turn","content":[]}}`
	}
	return t.appendTranscript(line)
}

// startToolCalls appends one assistant line per call, as claude writes them,
// and leaves the session busy so the calls stay in flight.
func (t *tui) startToolCalls(calls []ToolCall) error {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	var lines []string
	for i, c := range calls {
		line, err := json.Marshal(map[string]any{
			"isSidechain": false,
			"type":        "assistant",
			"timestamp":   ts,
			"message": map[string]any{
				"role":        "assistant",
				"stop_reason": "tool_use",
				"content": []map[string]any{{
					"type": "tool_use", "id": fmt.Sprintf("toolu_f%d", i+1), "name": c.Name, "input": c.Input,
				}},
			},
		})
		if err != nil {
			return err
		}
		lines = append(lines, string(line))
	}
	if err := t.appendTranscript(strings.Join(lines, "\n")); err != nil {
		return err
	}
	return t.setStatus(StatusBusy, "")
}

// appendUsage appends an assistant line whose whole context is tokens, as the
// last turn's usage reports it.
func (t *tui) appendUsage(tokens int) error {
	line, err := json.Marshal(map[string]any{
		"isSidechain": false,
		"type":        "assistant",
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
		"message": map[string]any{
			"role":        "assistant",
			"model":       t.opts.model,
			"stop_reason": "end_turn",
			"content":     []any{},
			"usage":       map[string]any{"input_tokens": tokens, "output_tokens": 1},
		},
	})
	if err != nil {
		return err
	}
	return t.appendTranscript(string(line))
}

func (t *tui) appendTranscript(lines string) error {
	if err := os.MkdirAll(filepath.Dir(t.transcript), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(t.transcript, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(lines + "\n")
	return err
}

// BackgroundTaskLabel is the description of the launching tool call of a
// BehaviourBackgroundTask task, which the gate card shows as its label.
const BackgroundTaskLabel = "run the soak"

// TaskReleaseSuffix names the file (Hold + TaskReleaseSuffix) whose removal ends
// the background task's process, like the hold file ends a held agent.
const TaskReleaseSuffix = ".task"

// TaskReleasePath is the release file of the background task for a worker hold
// file, or "" when the worker has no hold file.
func TaskReleasePath(hold string) string {
	if hold == "" {
		return ""
	}
	return hold + TaskReleaseSuffix
}

// startBackgroundTask records the launch of a background shell task and gives
// it what claude does: an output file under the task output base that a real
// child process holds open as its stdout.
func (t *tui) startBackgroundTask(hold string) error {
	const taskID, toolUseID = "b1", "toolu_b1"
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	launch := `{"isSidechain":false,"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","stop_reason":"tool_use","content":[{"type":"tool_use","id":"` + toolUseID + `","name":"Bash","input":{"description":"` + BackgroundTaskLabel + `","command":"make soak","run_in_background":true}}]}}`
	if err := t.appendTranscript(launch); err != nil {
		return err
	}
	dir := filepath.Join(backgroundtask.DefaultOutputBase(), "fixture", t.pid.SessionID, "tasks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out, err := os.Create(filepath.Join(dir, taskID+".output"))
	if err != nil {
		return err
	}
	defer out.Close()
	release := TaskReleasePath(hold)
	if release == "" {
		release = filepath.Join(dir, taskID+".release")
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		return err
	}
	// The child outlives this call and ends when the release file is removed,
	// like a task a flow lets finish. The bound keeps a forgotten one from lingering.
	script := `for i in 1 2 3 4; do echo soak line $i; done; n=0; while [ -e "$1" ] && [ $n -lt 750 ]; do sleep 0.2; n=$((n+1)); done`
	child := exec.Command("sh", "-c", script, "sh", release)
	child.Env = os.Environ()
	child.Stdout = out
	if err := child.Start(); err != nil {
		return err
	}
	// Like claude, a task that ends is reported resolved and the shell state clears.
	go func() {
		_ = child.Wait()
		_ = SendControl(t.ctlPath, Control{Action: ActionBackground})
		_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
	}()
	return t.writeBackgroundMarker(true)
}

// colored wraps s in red when the terminal supports color. Like claude
// (supports-color), it decides from TERM and COLORTERM alone, so a session
// without them is colorless.
func colored(s string) string {
	if !colorEnabled() {
		return s
	}
	return "\x1b[31m" + s + "\x1b[0m"
}

// draw clears the terminal and redraws the screen for the current state.
func (t *tui) draw() {
	title := IdleTitle
	var rows []string
	switch {
	case t.opts.trust:
		no, yes := "❯ No, exit", "  Yes, I trust this folder"
		if t.trustYes {
			no, yes = "  No, exit", "❯ Yes, I trust this folder"
		}
		rows = []string{"Quick safety check: Is this a project you created or one you trust?", "", no, yes}
	case t.localDialog != "":
		rows = t.localDialogRows()
	case t.compact != nil && t.pid.Status == StatusBusy:
		title = WorkingTitle
		rows = t.compactRows()
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
		title = WorkingTitle
		rows = []string{"> " + t.prompt, "✻ Working…"}
	case t.undecided:
		rows = t.undecidedRows()
	case t.opts.draft != "" || !t.opts.plainScreen:
		// The input box claudeui.InputBox recognises.
		rows = append([]string{colored("✻ Welcome to Claude Code"), ""}, t.landed...)
		rows = append(rows, t.boxRows()...)
	default:
		rows = []string{colored("✻ Welcome to Claude Code"), "", "> " + t.typed.String()}
	}
	if t.notice != "" && t.pid.Status == StatusIdle {
		for range max(t.noticeRows, 1) {
			rows = append(rows, t.notice)
		}
	}
	footer := t.opts.vimFooter
	if t.vim {
		footer = "-- INSERT --"
	}
	if footer != "" && t.pid.Status == StatusIdle {
		rows = append(rows, footer)
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
