package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"os"
	"strings"
	"time"
)

// Local-command events, one per line of TUILocalCommandsPath.
const (
	eventCompactStart = "compact-start"
	eventCompactDone  = "compact-done"
	eventModelSaved   = "model-saved"
	eventModelKept    = "model-kept"
	eventHelpClosed   = "help-closed"
)

const (
	dialogModel = "/model"
	dialogHelp  = "/help"

	compactDoneStdout = "Compacted (ctrl+o to see full summary)"
	modelSavedResult  = "Set model to Haiku 5.5"
	modelKeptResult   = "Kept model as Haiku 5.5"
	helpClosedResult  = "Help dialog dismissed"
)

// localEcho is the row claude draws for a command it ran locally: the prompt
// glyph on a grey chip at column 0, then the command in white. The bytes are
// the ones claude paints under COLORTERM=truecolor; the other colour depths
// follow the same switch as the input box.
func localEcho(command string) string {
	if !colorEnabled() {
		return "❯ " + command
	}
	switch os.Getenv("COLORTERM") {
	case "truecolor", "24bit":
		return "\x1b[38;2;153;153;153;48;2;55;55;55m❯\x1b[39m \x1b[38;2;255;255;255m" + command + "\x1b[39m"
	}
	return "\x1b[38;5;246;48;5;237m❯\x1b[39m \x1b[38;5;231m" + command + "\x1b[39m"
}

func (t *tui) logLocalCommand(event string) error {
	f, err := os.OpenFile(TUILocalCommandsPath(t.opts.home, t.opts.sessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(event + "\n")
	return err
}

// opensLocalDialog reports whether text is a bare /model or /help. Its launch
// record is not written: a local command's records come when it ends.
func (t *tui) opensLocalDialog(text string) bool {
	return text == dialogModel || text == dialogHelp
}

// openLocalDialog draws the /model or /help dialog. The session stays idle:
// that is the worst case, gx sees a promptable session and could type.
func (t *tui) openLocalDialog(command string) {
	t.localDialog = command
	t.draw()
}

func (t *tui) localDialogRows() []string {
	rows := []string{localEcho(t.localDialog), BoxRule()}
	if t.localDialog == dialogModel {
		return append(rows,
			"  Select model",
			"  Switch between Claude models. Applies to this session and future Claude Code sessions.",
			"    1.  Default (recommended)",
			"    2.  Opus 5.5",
			"    3.  Sonnet 5.5",
			"    4.  Fable 5.2",
			"  ❯ 5.  Haiku 5.5 ✔",
			"",
			"  s to use this session only · Enter to save · Esc to cancel",
		)
	}
	return append(rows,
		"  Help  General   Commands   Custom commands",
		"",
		"  Claude understands your codebase, makes edits with your permission, and executes commands.",
		"  Type / to see the available commands.",
		"",
		"  ←/→ to switch · Esc to close",
	)
}

// localDialogKey takes one key of an open dialog. Esc closes it, Enter saves
// the highlighted model; every other key is ignored.
func (t *tui) localDialogKey(key byte) error {
	command := t.localDialog
	switch {
	case key == EscKey:
		result, event := helpClosedResult, eventHelpClosed
		if command == dialogModel {
			result, event = modelKeptResult, eventModelKept
		}
		return t.closeLocalDialog(result, event)
	case key == '\r' && command == dialogModel:
		return t.closeLocalDialog(modelSavedResult, eventModelSaved)
	}
	return nil
}

// closeLocalDialog lands the command: its transcript records are written only
// now, and its echo and result stay above the box.
func (t *tui) closeLocalDialog(result, event string) error {
	command := t.localDialog
	t.localDialog = ""
	t.landed = []string{localEcho(command), "  ⎿  " + result}
	if err := t.logLocalCommand(event); err != nil {
		return err
	}
	if err := t.writeCommandOutput(command, result); err != nil {
		return err
	}
	t.draw()
	return nil
}

// compactRows is the frame of a running /compact: the box stays, empty.
func (t *tui) compactRows() []string {
	return []string{
		localEcho(t.compact.command),
		"✢ Compacting conversation… (1s · ↓ 54 tokens)",
		BoxRule(),
		"❯ ",
		BoxRule(),
		"  ⏵⏵ auto mode on (shift+tab to cycle) · esc to interrupt",
	}
}

// startCompact marks the turn that is about to go busy as a compaction. With
// TUICompactDurationEnv it ends by itself, otherwise only a control ends it.
func (t *tui) startCompact(command string) error {
	t.compact = &compactRun{command: command}
	if err := t.logLocalCommand(eventCompactStart); err != nil {
		return err
	}
	if d := t.opts.compactDuration; d > 0 {
		time.AfterFunc(d, func() { _ = SendControl(t.ctlPath, Control{Action: ActionFinish}) })
	}
	return nil
}

// finishCompact lands a running compaction; it does nothing when none runs.
func (t *tui) finishCompact() error {
	c := t.compact
	if c == nil {
		return nil
	}
	t.compact = nil
	t.landed = []string{"✻ Conversation compacted", localEcho(c.command), "  ⎿  " + compactDoneStdout}
	if err := t.logLocalCommand(eventCompactDone); err != nil {
		return err
	}
	return t.writeCommandOutput(c.command, compactDoneStdout)
}

type compactRun struct{ command string }

// undecidedRows is a frame the box reader cannot decide on: no box, and our
// text on a row that is neither the echo nor a menu row.
func (t *tui) undecidedRows() []string {
	return []string{colored("✻ Welcome to Claude Code"), "", "  " + t.typed.String()}
}

// armUndecided activates the undecided mode when the draft is a "/" command.
func (t *tui) armUndecided() bool {
	if t.undecidedNext && strings.HasPrefix(t.typed.String(), "/") {
		t.undecidedNext, t.undecided = false, true
	}
	return t.undecided
}
