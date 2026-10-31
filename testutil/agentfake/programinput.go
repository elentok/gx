package agentfake

import (
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

// Actions only the fake program's input box takes.
const (
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
	return filepath.Join(testDir(conversationID), conversationID+".submitted")
}

// programSubmittedPath is the same log, as the program itself finds it.
func programSubmittedPath(conversationID string) string {
	return filepath.Join(programDir(), conversationID+".submitted")
}

// ProgramInterruptEntry is what a Ctrl+C logs to ProgramSubmittedPath, so a
// test sees the key reach the program.
const ProgramInterruptEntry = "<ctrl-c>"

// promptMarker is colored like claude's, so even a plain draft has an SGR on
// screen: a read with no styling at all cannot tell a draft from a placeholder.
const promptMarker = "\x1b[35m❯\x1b[0m "

const normalModeText = "normal mode"

// inputBox is claude's input box: lines of a draft, the last one being edited.
type inputBox struct {
	out io.Writer
	// marker starts the first row: promptMarker unless a fake draws its own.
	marker    string
	lines     []string
	swallow   int
	vim       bool
	normal    bool
	submitted string
}

func newInputBox(out io.Writer, draft string) *inputBox {
	return &inputBox{out: out, marker: promptMarker, lines: strings.Split(draft, "\n")}
}

func (b *inputBox) text() string { return strings.Join(b.lines, "\n") }

func (b *inputBox) draw() {
	var sb strings.Builder
	sb.WriteString("\x1b[2J\x1b[H")
	if b.normal && b.text() == "" {
		// Unstyled text, so a draft check that reads the screen before the
		// INSERT switch sees a draft where the placeholder would be.
		sb.WriteString(b.marker + normalModeText)
	} else if b.text() == "" {
		sb.WriteString(b.marker + "\x1b[2m" + ProgramPlaceholder + "\x1b[0m")
	} else {
		for i, line := range b.lines {
			if i == 0 {
				sb.WriteString(b.marker + line)
			} else {
				sb.WriteString("\r\n  " + line)
			}
		}
	}
	if b.vim && !b.normal {
		sb.WriteString("\r\n-- INSERT --")
	}
	fmt.Fprint(b.out, sb.String())
}

// key applies a typed key and reports the text it submitted, if any.
func (b *inputBox) key(k byte) (submitted string, ok bool) {
	last := len(b.lines) - 1
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
		submitted = b.text()
		b.lines = []string{""}
		ok = true
	case k == '\n':
		b.lines = append(b.lines, "")
	case k == 0x15:
		// One press clears the line; the next removes the break before it.
		switch {
		case b.lines[last] != "":
			b.lines[last] = ""
		case last > 0:
			b.lines = b.lines[:last]
		}
	case k >= ' ' && k != 0x7f:
		// Appended bytewise, so a multi-byte rune is whole once its last byte lands.
		b.lines[last] += string([]byte{k})
	default:
		return "", false
	}
	b.draw()
	return submitted, ok
}

func (b *inputBox) control(c Control) bool {
	switch c.Action {
	case ActionSwallowEnter:
		b.swallow++
	case ActionVimNormal:
		b.vim, b.normal = true, true
		b.draw()
	default:
		return false
	}
	return true
}

func logSubmitted(conversationID, text string) error {
	f, err := os.OpenFile(programSubmittedPath(conversationID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%q\n", text)
	return err
}
