package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/agentrunner/claudeui"
)

// pasteChipOver is the size above which claude shows a paste as a chip.
const pasteChipOver = 512

// PastedChip is what claude draws for the nth paste of a draft, whose text
// holds lines line breaks. The research does not pin how claude counts M, so
// the table test and deliver-slash-policy read it from here.
func PastedChip(n int, text string) string {
	return fmt.Sprintf("[Pasted text #%d +%d lines]", n, PastedLines(text))
}

// PastedLines is the M of a paste chip, read from the one function the settle
// read uses too.
func PastedLines(text string) int { return claudeui.PastedLines(text) }

// hiddenRanges is claude's pinned table of characters it removes from a
// slash command or shell draft. It is written out here, not imported from
// agentrunner/deliver, so a bug in the strip cannot hide itself. The last
// range, U+1BCA0-U+1BCA3, is one claude cleans and gx's list lacks (drift).
var hiddenRanges = [][2]rune{
	{0x00, 0x08}, {0x0B, 0x1F}, {0x7F, 0x9F},
	{0xAD, 0xAD}, {0x34F, 0x34F}, {0x61C, 0x61C},
	{0x115F, 0x1160}, {0x17B4, 0x17B5}, {0x180B, 0x180F},
	{0x200B, 0x200F}, {0x202A, 0x202E}, {0x2060, 0x206F},
	{0x3164, 0x3164}, {0xFE00, 0xFE0F}, {0xFEFF, 0xFEFF},
	{0xFFA0, 0xFFA0}, {0xFFF0, 0xFFFB},
	{0x1BCA0, 0x1BCA3}, {0x1D173, 0x1D17A},
	{0xE0000, 0xE0FFF}, {0xE0100, 0xE01EF},
}

// joiner reports whether r is a ZWNJ, ZWJ or variation selector, which claude
// keeps between two non-space characters (emoji sequences, Persian words).
func joiner(r rune) bool {
	return r == 0x200C || r == 0x200D || (r >= 0xFE00 && r <= 0xFE0F)
}

// hiddenAt reports whether runes[i] is a hidden character.
func hiddenAt(runes []rune, i int) bool {
	r := runes[i]
	if joiner(r) && i > 0 && i+1 < len(runes) && !unicode.IsSpace(runes[i-1]) && !unicode.IsSpace(runes[i+1]) {
		return false
	}
	for _, rg := range hiddenRanges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// stripHidden removes the hidden characters of s and counts them.
func stripHidden(s string) (string, int) {
	runes := []rune(s)
	var sb strings.Builder
	removed := 0
	for i, r := range runes {
		if hiddenAt(runes, i) {
			removed++
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String(), removed
}

// invisibleNotice is the line claude draws after the Enter that cleaned a draft.
func invisibleNotice(n int) string {
	unit := "character"
	if n > 1 {
		unit = "characters"
	}
	return fmt.Sprintf("Removed %d invisible %s · review and press Enter to send", n, unit)
}

// slashMenu are the commands the suggestion menu offers.
var slashMenu = []struct{ name, desc string }{
	{"/help", "Show help and available commands"},
	{"/model", "Set the AI model for Claude Code"},
	{"/compact", "Clear conversation history but keep a summary"},
	{"/clear", "Clear conversation history and free up context"},
	{"/exit", "Exit the REPL"},
	{"/cost", "Show the total cost and duration of the session"},
	{"/copy", "Copy Claude's last response to clipboard"},
}

// knownCommand reports whether token ("/name") is a command claude highlights.
func (t *tui) knownCommand(token string) bool {
	if _, hidden := stripHidden(token); hidden > 0 {
		return false
	}
	for _, c := range slashMenu {
		if c.name == token {
			return true
		}
	}
	return len(token) > 1 && !t.opts.skillUnknown(token, time.Since(t.started))
}

// colorEnabled decides from TERM and COLORTERM alone, like claude
// (supports-color), so a session without them is colorless.
func colorEnabled() bool {
	term := os.Getenv("TERM")
	return os.Getenv("COLORTERM") != "" || (term != "" && term != "dumb")
}

// commandColor is the SGR claude paints a known slash command with.
func commandColor() string {
	switch os.Getenv("COLORTERM") {
	case "truecolor", "24bit":
		return "\x1b[38;2;177;185;249m"
	}
	return "\x1b[38;5;147m"
}

// menuRows is the suggestion menu under the box: the known commands that
// start with the draft while it is one "/..." token. Esc closes it until the
// next typed key.
func (t *tui) menuRows() []string {
	draft := t.typed.String()
	if t.menuClosed || t.pid.Status != StatusIdle || !strings.HasPrefix(draft, "/") || strings.ContainsAny(draft, " \n") {
		return nil
	}
	var rows []string
	for _, c := range slashMenu {
		if !strings.HasPrefix(c.name, draft) {
			continue
		}
		lead := "    "
		if len(rows) == 0 {
			lead = "  ❯ "
		}
		rows = append(rows, lead+c.name+"   "+c.desc)
	}
	return rows
}

// boxRows is the idle input box: rule, draft rows, rule, then the menu.
func (t *tui) boxRows() []string {
	rule := BoxRule()
	width := max(BoxCols()-2, 1)
	prefix := "❯ "
	var draft []string
	text := t.typed.String()
	if text == "" {
		draft = []string{prefix + "\x1b[2m" + ProgramPlaceholder + "\x1b[0m"}
	} else {
		text = strings.NewReplacer(" ", "\n", " ", "\n").Replace(text)
		for i, line := range strings.Split(text, "\n") {
			if i == 0 {
				line = t.highlight(line)
			}
			for j, row := range strings.Split(ansi.Wrap(line, width, ""), "\n") {
				if i == 0 && j == 0 {
					// Real claude always styles the box, so a plain draft is still a
					// styled read; with no SGR anywhere the harness check is undecided.
					row = prefix + row + "\x1b[39m"
				} else {
					row = "  " + row
				}
				draft = append(draft, row)
			}
		}
	}
	rows := append([]string{rule}, draft...)
	rows = append(rows, rule)
	return append(rows, t.menuRows()...)
}

// highlight paints the command token of a draft's first line when it is a
// known command and the terminal has colour.
func (t *tui) highlight(line string) string {
	if !colorEnabled() || !strings.HasPrefix(line, "/") {
		return line
	}
	token, rest, _ := strings.Cut(line, " ")
	if !t.knownCommand(token) {
		return line
	}
	if rest != "" || strings.HasSuffix(line, " ") {
		rest = " " + rest
	}
	return commandColor() + token + "\x1b[m" + rest
}

// expandPastes puts the real text back for the chips of the draft.
func (t *tui) expandPastes(text string) string {
	for i, p := range t.pastes {
		text = strings.Replace(text, PastedChip(i+1, p), p, 1)
	}
	return text
}
