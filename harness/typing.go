package harness

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/elentok/gx/agentrunner"
)

// Codex is the codex harness's name. It has a typing part only: the herdr
// runner still launches it through its kind switch.
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
	// start; false means give up.
	ReadyToType() bool
}

// LookupTyping returns the typing rules for kind.
func LookupTyping(kind agentrunner.Kind) (Typing, error) {
	switch kind {
	case Claude:
		return claudeHarness{}, nil
	case Codex:
		return codexTyping{}, nil
	}
	return nil, fmt.Errorf("no typing rules for %q", kind)
}

const ctrlU = "\x15"

func (claudeHarness) InputIsEmpty(styledRows []string) InputState {
	return inputState(styledRows, "❯", false)
}

func (claudeHarness) ClearKey() string { return ctrlU }

func (claudeHarness) ReadyToType() bool { return true }

type codexTyping struct{}

func (codexTyping) InputIsEmpty(styledRows []string) InputState {
	return inputState(styledRows, "›", true)
}

func (codexTyping) ClearKey() string { return ctrlU }

func (codexTyping) ReadyToType() bool { return true }

// inputState finds the last row that starts with marker (bold when
// boldMarker) and decides from the styling of the text after it and of the
// draft rows that continue it.
func inputState(rows []string, marker string, boldMarker bool) InputState {
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
	start := -1
	for i, row := range rows {
		cells := parseCells(row)
		if len(cells) > 0 && string(cells[0].r) == marker && (!boldMarker || cells[0].style.bold) {
			start = i
		}
	}
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
// first placeholder rune or past the end of a draft.
func hasDraftText(cells []cell) bool {
	for _, c := range cells {
		if c.r != ' ' && !c.style.dim && !c.style.reverse {
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
