package agentfake

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ProgramInputEnv, set to "1", gives the fake program an input box like
// claude's: a prompt marker, a dim placeholder, multi-line drafts. Without it
// the program echoes typed keys raw.
const ProgramInputEnv = "GX_AGENTFAKE_PROGRAM_INPUT"

// ProgramDraftEnv seeds the input box with a draft; "\n" in it breaks a line.
const ProgramDraftEnv = "GX_AGENTFAKE_PROGRAM_DRAFT"

// ProgramReadyDelayEnv is a Go duration the input box takes to appear. Keys
// typed before it are dropped, as claude drops them.
const ProgramReadyDelayEnv = "GX_AGENTFAKE_PROGRAM_READY_DELAY"

// ProgramInsertNoMarkerEnv, set to "1", puts the input box in vim INSERT mode
// without drawing the "-- INSERT --" footer row.
const ProgramInsertNoMarkerEnv = "GX_AGENTFAKE_PROGRAM_INSERT_NO_MARKER"

// ProgramClearLeavesIEnv, set to "1", makes the first clear key leave a lone
// "i" in the input; the second clears it. It also logs every clear key.
const ProgramClearLeavesIEnv = "GX_AGENTFAKE_PROGRAM_CLEAR_LEAVES_I"

// ProgramLogClearEnv, set to "1", logs every clear key (Ctrl+U) to
// ProgramSubmittedPath as ProgramClearEntry, so a test counts them. It is off
// by default because other tests compare that log exactly. Pre-typed text
// needs no knob of its own: ProgramDraftEnv seeds a lone "i" or a draft.
const ProgramLogClearEnv = "GX_AGENTFAKE_PROGRAM_LOG_CLEAR"

// ProgramClearEntry is what a clear key logs under ProgramLogClearEnv.
const ProgramClearEntry = "<ctrl-u>"

// Actions only the fake program's input box takes.
const (
	// ActionVimToggle switches an input box in normal editing mode to vim INSERT,
	// as a /config toggle would, and writes editorMode vim to the settings file
	// of CLAUDE_CONFIG_DIR.
	ActionVimToggle = "vim-toggle"
	// ActionSwallowEnter makes one more Enter do nothing, draft kept; sending it
	// twice swallows two.
	ActionSwallowEnter = "swallow-enter"
	// ActionVimNormal turns vim mode on and enters NORMAL: no INSERT marker,
	// and keys other than "i" are not inserted.
	ActionVimNormal = "vim-normal"
)

// ProgramPlaceholder is the dim text of an empty input box.
const ProgramPlaceholder = `Try "fix the failing test"`

// ProgramClearCommand is the local slash command that changes the
// conversation id without a turn.
const ProgramClearCommand = "/clear"

// ProgramClearedSuffix is appended to the conversation id by
// ProgramClearCommand.
const ProgramClearedSuffix = "-cleared"

// ProgramSubmittedPath is the log of every submitted input of the fake
// program running conversationID, one %q-quoted entry per line.
func ProgramSubmittedPath(conversationID string) string {
	return filepath.Join(TestDir(conversationID), conversationID+".submitted")
}

// programSubmittedPath is the same log, as the program itself finds it.
func programSubmittedPath(conversationID string) string {
	return filepath.Join(ProgramDir(), conversationID+".submitted")
}

// ProgramInterruptEntry is what a Ctrl+C logs to ProgramSubmittedPath, so a
// test sees the key reach the program.
const ProgramInterruptEntry = "<ctrl-c>"

// ProgramEscEntry is what an Esc logs to ProgramSubmittedPath. It is logged
// once the next key arrives, so a test sends a second key after it. The other
// non-text keys log "<tab>", "<backspace>" and "<csi X>" for an arrow or
// Shift+Tab with final byte X.
const ProgramEscEntry = "<esc>"

// promptMarker is colored like claude's, so even a plain draft has an SGR on
// screen: a read with no styling at all cannot tell a draft from a placeholder.
const promptMarker = "\x1b[35m❯\x1b[0m "

// InputBox is claude's input box: lines of a draft, the last one being edited.
type InputBox struct {
	out io.Writer
	// Marker starts the first row: promptMarker unless a fake draws its own.
	Marker    string
	Lines     []string
	swallow   int
	pasting   bool
	vim       bool
	normal    bool
	submitted string
	// noMarker leaves the INSERT footer undrawn.
	noMarker bool
	// leaveI makes the next clear leave a lone "i"; logClear makes program log
	// each clear key.
	leaveI, logClear bool
	// configPath is the settings file ActionVimToggle writes.
	configPath string
}

func NewInputBox(out io.Writer, draft string) *InputBox {
	return &InputBox{out: out, Marker: promptMarker, Lines: strings.Split(draft, "\n")}
}

func (b *InputBox) Text() string { return strings.Join(b.Lines, "\n") }

// pasteMarker is the bracketed-paste marker a CSI sequence with these params
// and final byte is, if it is one.
func pasteMarker(params string, final byte) (string, bool) {
	switch {
	case final != '~':
		return "", false
	case params == "200":
		return pasteStart, true
	case params == "201":
		return pasteEnd, true
	}
	return "", false
}

// Mark keeps a bracketed-paste marker in the input, so the submitted log shows
// where each paste began and ended. Draw leaves markers off the screen.
func (b *InputBox) Mark(marker string) {
	b.Lines[len(b.Lines)-1] += marker
	// A long paste would redraw the whole input once per byte; it draws once
	// the paste ends.
	b.pasting = marker == pasteStart
	if !b.pasting {
		b.Draw()
	}
}

var pasteMarkers = strings.NewReplacer(pasteStart, "", pasteEnd, "")

func (b *InputBox) Draw() {
	var sb strings.Builder
	sb.WriteString("\x1b[2J\x1b[H")
	if b.normal && b.Text() == "" {
		// The placeholder's text, unstyled, so a draft check that reads the
		// screen before the INSERT switch sees a draft, while the vim probe sees
		// the same text before and after its `i`.
		sb.WriteString(b.Marker + ProgramPlaceholder)
	} else if b.Text() == "" {
		sb.WriteString(b.Marker + "\x1b[2m" + ProgramPlaceholder + "\x1b[0m")
	} else {
		for i, line := range b.Lines {
			line = pasteMarkers.Replace(line)
			if i == 0 {
				sb.WriteString(b.Marker + line)
			} else {
				sb.WriteString("\r\n  " + line)
			}
		}
	}
	if b.vim && !b.normal && !b.noMarker {
		sb.WriteString("\r\n-- INSERT --")
	}
	fmt.Fprint(b.out, sb.String())
}

// Key applies a typed Key and reports the text it submitted, if any.
func (b *InputBox) Key(k byte) (submitted string, ok bool) {
	last := len(b.Lines) - 1
	switch {
	case b.normal:
		if k == 'i' {
			b.normal = false
		}
	case k == '\r':
		if b.swallow > 0 {
			b.swallow--
			return "", false
		}
		submitted = b.Text()
		b.Lines = []string{""}
		ok = true
	case k == '\n':
		b.Lines = append(b.Lines, "")
	case k == 0x15:
		// One press clears the line; the next removes the break before it.
		switch {
		case b.leaveI:
			b.leaveI = false
			b.Lines = []string{"i"}
		case b.Lines[last] != "":
			b.Lines[last] = ""
		case last > 0:
			b.Lines = b.Lines[:last]
		}
	case k == 0x7f:
		if line := b.Lines[last]; line != "" {
			b.Lines[last] = line[:len(line)-1]
		}
	case k >= ' ' && k != 0x7f:
		// Appended bytewise, so a multi-byte rune is whole once its last byte lands.
		b.Lines[last] += string([]byte{k})
	default:
		return "", false
	}
	if !b.pasting {
		b.Draw()
	}
	return submitted, ok
}

func (b *InputBox) control(c Control) bool {
	switch c.Action {
	case ActionSwallowEnter:
		b.swallow++
	case ActionVimNormal:
		b.vim, b.normal = true, true
		b.Draw()
	case ActionVimToggle:
		b.vim, b.normal = true, false
		if err := b.writeEditorMode(); err != nil {
			fmt.Fprintln(os.Stderr, "fakeclaude:", err)
		}
		b.Draw()
	default:
		return false
	}
	return true
}

// writeEditorMode records editorMode vim in the settings file, keeping its
// other keys, where launchEditorMode (and gx's claudeVimMode) read it.
func (b *InputBox) writeEditorMode() error { return writeVimSetting(b.configPath) }

// writeVimSetting records editorMode vim in the settings file at path.
func writeVimSetting(path string) error {
	settings := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &settings)
	}
	settings["editorMode"] = "vim"
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func LogSubmitted(conversationID, text string) error {
	f, err := os.OpenFile(programSubmittedPath(conversationID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%q\n", text)
	return err
}
