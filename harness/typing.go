package harness

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/elentok/gx/agentrunner"
)

// Codex is the codex harness's name (see codexHarness and LookupTyping).
const Codex agentrunner.Kind = "codex"

// InputState is what a screen read says about the agent's input box.
type InputState int

const (
	// InputUndecided is a read that can't tell, such as one with no styling.
	InputUndecided InputState = iota
	InputEmpty
	InputNotEmpty
)

// Typing is the part of a harness that the typing paths call: they never look
// at an agent's screen themselves.
type Typing interface {
	// InputIsEmpty reads the input box from styled rows (ANSI SGR
	// sequences kept). The placeholder is told apart from a draft by its dim
	// style, never by its words, so a read without styling is InputUndecided.
	InputIsEmpty(styledRows []string) InputState
	// ClearKey is the key a full clear repeats.
	ClearKey() string
	// ReadyToType runs before any typed text and reports whether typing can
	// start; false means give up. It may press keys and re-read through io.
	ReadyToType(io TypingIO) bool
}

// TypingIO is how a ready-to-type step reads the agent's screen and presses
// keys, whichever runner owns the session. Rows may be styled or plain.
type TypingIO struct {
	Rows     func() ([]string, error)
	SendKeys func(keys string) error
}

// LookupTyping returns the typing rules for kind.
func LookupTyping(kind agentrunner.Kind) (Typing, error) {
	switch kind {
	case Claude:
		return claudeHarness{}, nil
	case Codex:
		return codexTyping{codexInput}, nil
	}
	return nil, fmt.Errorf("no typing rules for %q", kind)
}

const ctrlU = "\x15"

// ClaudeTyping is the typing rules of the claude harness, the only one the
// server and the PTY runner type into.
func ClaudeTyping() Typing { return claudeHarness{} }

// ClaudeTypingIn is ClaudeTyping reading claude's config from env (see
// LookupEnv).
func ClaudeTypingIn(env []string) Typing { return claudeHarness{env: env} }

// markerTyping is the typing part shared by harnesses whose input row starts
// with a marker rune.
type markerTyping struct {
	marker string
	bold   bool
}

var (
	claudeInput = markerTyping{marker: "❯"}
	codexInput  = markerTyping{marker: "›", bold: true}
)

func (m markerTyping) InputIsEmpty(styledRows []string) InputState {
	return inputState(styledRows, m)
}

func (markerTyping) ClearKey() string { return ctrlU }

func (claudeHarness) InputIsEmpty(styledRows []string) InputState {
	return claudeInput.InputIsEmpty(styledRows)
}

func (claudeHarness) ClearKey() string { return claudeInput.ClearKey() }

type codexTyping struct{ markerTyping }

func (codexTyping) ReadyToType(TypingIO) bool { return true }

// lastInputRow is the index of the last row that starts with the marker (bold
// when m.bold), or -1.
func lastInputRow(rows []string, m markerTyping) int {
	start := -1
	for i, row := range rows {
		cells := parseCells(row)
		if len(cells) > 0 && string(cells[0].r) == m.marker && (!m.bold || cells[0].style.bold) {
			start = i
		}
	}
	return start
}

// inputState decides from the styling of the text after the last input row's
// marker and of the draft rows that continue it.
func inputState(rows []string, m markerTyping) InputState {
	styled := false
	for _, row := range rows {
		if strings.Contains(row, "\x1b[") {
			styled = true
			break
		}
	}
	if !styled {
		return InputUndecided
	}
	start := lastInputRow(rows, m)
	if start < 0 {
		return InputUndecided
	}
	// The first row's text follows the marker; a multi-line draft continues on
	// rows indented under it, up to the first row that isn't.
	if hasDraftText(parseCells(rows[start])[1:]) {
		return InputNotEmpty
	}
	for _, row := range rows[start+1:] {
		if !strings.HasPrefix(plain(row), "  ") {
			break
		}
		if hasDraftText(parseCells(row)) {
			return InputNotEmpty
		}
	}
	return InputEmpty
}

// hasDraftText reports whether cells hold a visible rune that is neither dim
// nor reverse video. Reverse video is the cursor block, which sits on the
// first placeholder rune or past the end of a draft. Any space counts as
// blank: Claude draws a no-break space after its marker.
func hasDraftText(cells []cell) bool {
	for _, c := range cells {
		if !unicode.IsSpace(c.r) && !c.style.dim && !c.style.reverse {
			return true
		}
	}
	return false
}

type sgr struct{ bold, dim, reverse bool }

type cell struct {
	r     rune
	style sgr
}

func plain(row string) string {
	var sb strings.Builder
	for _, c := range parseCells(row) {
		sb.WriteRune(c.r)
	}
	return sb.String()
}

// parseCells splits a row into runes with the SGR state each was drawn in.
// Other escape sequences are dropped.
func parseCells(row string) []cell {
	var cells []cell
	var st sgr
	rs := []rune(row)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\x1b' || i+1 >= len(rs) || rs[i+1] != '[' {
			cells = append(cells, cell{rs[i], st})
			continue
		}
		j := i + 2
		for j < len(rs) && (rs[j] < '@' || rs[j] > '~') {
			j++
		}
		if j < len(rs) && rs[j] == 'm' {
			st = applySGR(st, string(rs[i+2:j]))
		}
		i = j
	}
	return cells
}

func applySGR(st sgr, params string) sgr {
	if params == "" {
		return sgr{}
	}
	for p := range strings.SplitSeq(params, ";") {
		n, _ := strconv.Atoi(p)
		switch n {
		case 0:
			st = sgr{}
		case 1:
			st.bold = true
		case 2:
			st.dim = true
		case 7:
			st.reverse = true
		case 22:
			st.bold, st.dim = false, false
		case 27:
			st.reverse = false
		}
	}
	return st
}
