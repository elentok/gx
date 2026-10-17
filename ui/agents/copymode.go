package agents

import (
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

var (
	styleCopyCursor    = lipgloss.NewStyle().Reverse(true)
	styleCopySelection = lipgloss.NewStyle().Background(ui.ColorSurface1)
)

// copyMode is keyboard copy mode over the attach's frames. Positions are
// nativerunner.LinePos: lines count from the oldest line ever pushed to the
// scrollback, so they stay put while the view scrolls and the scrollback
// trims.
type copyMode struct {
	// a is the attach copy mode was entered on; another attach ends it.
	a      *attachment
	cursor nativerunner.LinePos
	// anchor is the selection's other end, set while selecting.
	anchor    nativerunner.LinePos
	selecting bool
	linewise  bool
	// pendingG: the first g of `g g` was pressed.
	pendingG bool
}

// enterCopyMode starts copy mode on the bottom-most non-blank row in view.
func (m Model) enterCopyMode() (tea.Model, tea.Cmd) {
	a := m.attach
	if a == nil || a.stream == nil || a.ended {
		return m, nil
	}
	c := &copyMode{a: a}
	top, h := m.copyView()
	c.cursor.Line = top
	for i := h - 1; i >= 0; i-- {
		if strings.TrimSpace(m.copyLineText(top+i)) != "" {
			c.cursor.Line = top + i
			break
		}
	}
	m.copy = c
	return m, nil
}

// copyView is the view's top line and its height in rows.
func (m Model) copyView() (top, height int) {
	_, height = m.screenSize()
	a := m.attach
	return a.scrollBase + a.scrollMax - a.offset, height
}

// copyLineText is the plain text of a line in view, "" for one out of view.
func (m Model) copyLineText(line int) string {
	top, _ := m.copyView()
	if i := line - top; i >= 0 && i < len(m.attach.rows) {
		return ansi.Strip(m.attach.rows[i])
	}
	return ""
}

func (m Model) updateCopyKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := m.copy
	if c.a != m.attach || c.a.ended {
		m.copy = nil
		return m, nil
	}
	k := msg.String()
	if c.pendingG {
		c.pendingG = false
		if k == "g" && c.a.mouse {
			// The child's history has no known top to wheel to.
			c.cursor = nativerunner.LinePos{Line: c.a.scrollBase + c.a.scrollMax - c.a.offset}
		} else if k == "g" {
			c.cursor = nativerunner.LinePos{Line: c.a.scrollBase}
			m.scrollToCopyCursor()
		}
		return m, nil
	}
	top, h := m.copyView()
	last := c.a.scrollBase + c.a.scrollMax + h - 1
	switch k {
	case "h", "left":
		c.cursor.Col = max(c.cursor.Col-1, 0)
	case "l", "right":
		cols, _ := m.screenSize()
		c.cursor.Col = min(c.cursor.Col+1, cols-1)
	case "j", "down":
		c.cursor.Line++
	case "k", "up":
		c.cursor.Line--
	case "0":
		c.cursor.Col = 0
	case "$":
		c.cursor.Col = max(ansi.StringWidth(strings.TrimRight(m.copyLineText(c.cursor.Line), " "))-1, 0)
	case "w", "b", "e":
		c.cursor = wordMove(k, c.cursor, top, top+h-1, m.copyLineText)
	case "ctrl+d":
		c.cursor.Line += h / 2
	case "ctrl+u":
		c.cursor.Line -= h / 2
	case "pgdown":
		c.cursor.Line += h
	case "pgup":
		c.cursor.Line -= h
	case "g":
		c.pendingG = true
		return m, nil
	case "G":
		c.cursor = nativerunner.LinePos{Line: last}
	case "v", "V":
		linewise := k == "V"
		if c.selecting && c.linewise == linewise {
			c.selecting = false
		} else {
			if !c.selecting {
				c.anchor = c.cursor
			}
			c.selecting, c.linewise = true, linewise
		}
		return m, nil
	case "y", "enter":
		m.copySelection()
		return m.leaveCopyMode(), nil
	case "q":
		return m.leaveCopyMode(), nil
	case "esc":
		if c.selecting {
			c.selecting = false
			return m, nil
		}
		return m.leaveCopyMode(), nil
	default:
		return m, nil
	}
	if c.a.mouse {
		m.wheelToCopyCursor()
		return m, nil
	}
	c.cursor.Line = max(c.a.scrollBase, min(c.cursor.Line, last))
	m.scrollToCopyCursor()
	return m, nil
}

// wheelToCopyCursor keeps the cursor on screen for a child that reports the
// mouse, such as claude in its fullscreen mode: its history lives in the
// child, not the holder's scrollback, so a move past an edge turns the
// child's wheel, one tick per line. A selection pins the child, since the
// text under it would move. Ticks go wheelTickGap apart, as claude speeds up
// a burst of them.
func (m Model) wheelToCopyCursor() {
	c := m.copy
	top, h := m.copyView()
	over := 0
	if c.cursor.Line < top {
		over = c.cursor.Line - top
	} else if bottom := top + h - 1; c.cursor.Line > bottom {
		over = c.cursor.Line - bottom
	}
	c.cursor.Line -= over
	if over == 0 || c.selecting {
		return
	}
	x, y, down := c.cursor.Col, c.cursor.Line-top, over > 0
	for i := range max(over, -over) {
		c.a.write(func(s AttachStream) error {
			if i > 0 {
				time.Sleep(wheelTickGap)
			}
			return s.Wheel(x, y, down)
		})
	}
}

// wheelTickGap is the least gap at which claude scrolls a line per tick.
const wheelTickGap = 40 * time.Millisecond

// scrollToCopyCursor scrolls the view through the holder just enough to show
// the cursor.
func (m Model) scrollToCopyCursor() {
	a, line := m.attach, m.copy.cursor.Line
	top, h := m.copyView()
	offset := a.offset
	if line < top {
		offset = a.scrollBase + a.scrollMax - line
	} else if line >= top+h {
		offset = a.scrollBase + a.scrollMax - (line - h + 1)
	}
	if offset = max(0, min(offset, a.scrollMax)); offset != a.offset {
		a.scroll(offset)
	}
}

// selection is the selected range, end exclusive as the holder takes it.
func (c *copyMode) selection() (start, end nativerunner.LinePos) {
	start, end = c.anchor, c.cursor
	if end.Line < start.Line || (end.Line == start.Line && end.Col < start.Col) {
		start, end = end, start
	}
	if c.linewise {
		// The end column reaches past any line; the holder clamps it.
		return nativerunner.LinePos{Line: start.Line}, nativerunner.LinePos{Line: end.Line, Col: 1 << 30}
	}
	end.Col++
	return start, end
}

// copySelection asks the holder for the selected text; its reply goes to
// the clipboard in updateAttachStream. Without a selection nothing is copied.
func (m Model) copySelection() {
	if m.copy.selecting {
		start, end := m.copy.selection()
		m.copy.a.write(func(s AttachStream) error { return s.Copy(start, end) })
	}
}

func (m Model) leaveCopyMode() Model {
	if a := m.copy.a; a == m.attach && !a.ended {
		a.snapToBottom()
	}
	m.copy = nil
	return m
}

// writeCopied puts the holder's copy reply on the clipboard.
func (m Model) writeCopied(text string) tea.Cmd {
	if err := m.clipWrite(text); err != nil {
		return notify.Error("copy failed: " + err.Error())
	}
	return notify.Success("copied")
}

// copyRows draws the cursor and the selection over the rows in view.
func (m Model) copyRows(rows []string) []string {
	c := m.copy
	top, _ := m.copyView()
	cols, _ := m.screenSize()
	var start, end nativerunner.LinePos
	if c.selecting {
		start, end = c.selection()
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		line := top + i
		if c.selecting && line >= start.Line && line <= end.Line {
			from, to := 0, cols
			if line == start.Line {
				from = start.Col
			}
			if line == end.Line {
				to = min(end.Col, cols)
			}
			row = overlayCells(row, from, to, styleCopySelection)
		}
		if line == c.cursor.Line {
			row = overlayCells(row, c.cursor.Col, c.cursor.Col+1, styleCopyCursor)
		}
		out[i] = row
	}
	return out
}

// overlayCells restyles the cells from up to to of an ANSI row, padding a
// short row with spaces.
func overlayCells(row string, from, to int, style lipgloss.Style) string {
	if w := ansi.StringWidth(row); w < to {
		row += strings.Repeat(" ", to-w)
	}
	if from >= to {
		return row
	}
	return ansi.Cut(row, 0, from) + style.Render(ansi.Strip(ansi.Cut(row, from, to))) + ansi.Cut(row, to, ansi.StringWidth(row))
}

// wordMove is vi's w, b or e over the lines from first to last. A line's
// end counts as a space, so words never run across lines.
func wordMove(k string, pos nativerunner.LinePos, first, last int, text func(int) string) nativerunner.LinePos {
	lines := make([][]rune, last-first+1)
	for i := range lines {
		lines[i] = []rune(text(first + i))
	}
	// p indexes lines[line] with col == len as the line's end.
	type p struct{ line, col int }
	cur := p{max(0, min(pos.Line-first, last-first)), pos.Col}
	class := func(q p) int {
		l := lines[q.line]
		if q.col >= len(l) {
			return 0
		}
		return runeClass(l[q.col])
	}
	step := func(q p, dir int) (p, bool) {
		q.col = min(q.col, len(lines[q.line]))
		q.col += dir
		switch {
		case q.col > len(lines[q.line]):
			if q.line == len(lines)-1 {
				return q, false
			}
			return p{q.line + 1, 0}, true
		case q.col < 0:
			if q.line == 0 {
				return q, false
			}
			return p{q.line - 1, len(lines[q.line-1])}, true
		}
		return q, true
	}
	switch k {
	case "w":
		q := cur
		for c := class(q); c != 0 && class(q) == c; {
			var ok bool
			if q, ok = step(q, 1); !ok {
				return pos
			}
		}
		for class(q) == 0 {
			var ok bool
			if q, ok = step(q, 1); !ok {
				return pos
			}
		}
		cur = q
	case "e", "b":
		dir := 1
		if k == "b" {
			dir = -1
		}
		q, ok := step(cur, dir)
		for ok && class(q) == 0 {
			q, ok = step(q, dir)
		}
		if !ok {
			return pos
		}
		for c := class(q); ; {
			n, ok := step(q, dir)
			if !ok || class(n) != c {
				break
			}
			q = n
		}
		cur = q
	}
	return nativerunner.LinePos{Line: first + cur.line, Col: cur.col}
}

func runeClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	}
	return 2
}
