package terminal

import (
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

var (
	styleCopyCursor    = lipgloss.NewStyle().Reverse(true)
	StyleCopySelection = lipgloss.NewStyle().Background(ui.ColorSurface1)
)

// ScreenInfo is the screen copy mode works over. Lines count as
// nativerunner.LinePos does; top is the line shown in the first row.
type ScreenInfo struct {
	cols, height int
	top          int
	// base is the oldest line kept; scrollMax is how far the view can scroll
	// back, and offset how far it is scrolled now.
	base, scrollMax, offset int
	rows                    []string
	// mouse: the child reports the mouse, so its history is not the holder's.
	mouse bool
}

// ScreenSource is what copy mode reads from and drives on the attach.
type ScreenSource interface {
	Screen() ScreenInfo
	scroll(offset int)
	write(f func(AttachStream) error)
	snapToBottom()
}

// AttachScreen is an attachment seen as a screenSource at the panel's size.
type AttachScreen struct {
	*Attachment
	cols, rows int
}

// NewAttachScreen is a at the panel's size.
func NewAttachScreen(a *Attachment, cols, rows int) AttachScreen {
	return AttachScreen{Attachment: a, cols: cols, rows: rows}
}

func (s AttachScreen) Screen() ScreenInfo {
	a := s.Attachment
	return ScreenInfo{
		cols: s.cols, height: s.rows,
		top:  a.scrollBase + a.scrollMax - a.offset,
		base: a.scrollBase, scrollMax: a.scrollMax, offset: a.offset,
		rows: a.Rows, mouse: a.mouse,
	}
}

// last is the last line the view can show.
func (s ScreenInfo) last() int { return s.base + s.scrollMax + s.height - 1 }

// lineText is the plain text of a line in view, "" for one out of view.
func (s ScreenInfo) lineText(line int) string {
	if i := line - s.top; i >= 0 && i < len(s.rows) {
		return ansi.Strip(s.rows[i])
	}
	return ""
}

// CopyMode is keyboard copy mode over the attach's frames. Positions are
// nativerunner.LinePos: lines count from the oldest line ever pushed to the
// scrollback, so they stay put while the view scrolls and the scrollback
// trims.
type CopyMode struct {
	// A is the attach copy mode was entered on; another attach ends it. Only
	// compared, never read through: the screenSource does the reading.
	A      *Attachment
	Cursor holder.LinePos
	// anchor is the selection's other end, set while selecting.
	anchor    holder.LinePos
	Selecting bool
	linewise  bool
	// pendingG: the first g of `g g` was pressed.
	pendingG bool
}

// CopyResult tells the tab what a key did.
type CopyResult struct {
	// Done: copy mode ended.
	Done bool
}

// NewCopyMode starts copy mode on the bottom-most non-blank row in view.
func NewCopyMode(a *Attachment, sc ScreenInfo) *CopyMode {
	c := &CopyMode{A: a}
	c.Cursor.Line = sc.top
	for i := sc.height - 1; i >= 0; i-- {
		if strings.TrimSpace(sc.lineText(sc.top+i)) != "" {
			c.Cursor.Line = sc.top + i
			break
		}
	}
	return c
}

// Update handles a key in copy mode.
func (c *CopyMode) Update(msg tea.KeyPressMsg, src ScreenSource) (*CopyMode, tea.Cmd, CopyResult) {
	k := msg.String()
	sc := src.Screen()
	switch {
	case c.pendingG:
		c.pendingG = false
		c.goTop(k, src, sc)
	case c.move(k, sc):
		c.follow(src, sc)
	case k == "g":
		c.pendingG = true
	case k == "v" || k == "V":
		c.toggleSelect(k == "V")
	default:
		return c, nil, c.exitKey(k, src)
	}
	return c, nil, CopyResult{}
}

// goTop finishes `g g`; any other second key drops the g.
func (c *CopyMode) goTop(k string, src ScreenSource, sc ScreenInfo) {
	if k != "g" {
		return
	}
	if sc.mouse {
		// The child's history has no known top to wheel to.
		c.Cursor = holder.LinePos{Line: sc.top}
		return
	}
	c.Cursor = holder.LinePos{Line: sc.base}
	c.scrollToCursor(src, sc)
}

// move applies a cursor motion key, reporting whether k was one.
func (c *CopyMode) move(k string, sc ScreenInfo) bool {
	h := sc.height
	switch k {
	case "h", "left":
		c.Cursor.Col = max(c.Cursor.Col-1, 0)
	case "l", "right":
		c.Cursor.Col = min(c.Cursor.Col+1, sc.cols-1)
	case "j", "down":
		c.Cursor.Line++
	case "k", "up":
		c.Cursor.Line--
	case "0":
		c.Cursor.Col = 0
	case "$":
		c.Cursor.Col = max(ansi.StringWidth(strings.TrimRight(sc.lineText(c.Cursor.Line), " "))-1, 0)
	case "w", "b", "e":
		c.Cursor = wordMove(k, c.Cursor, sc.top, sc.top+h-1, sc.lineText)
	case "ctrl+d":
		c.Cursor.Line += h / 2
	case "ctrl+u":
		c.Cursor.Line -= h / 2
	case "pgdown":
		c.Cursor.Line += h
	case "pgup":
		c.Cursor.Line -= h
	case "G":
		c.Cursor = holder.LinePos{Line: sc.last()}
	default:
		return false
	}
	return true
}

// follow keeps the cursor in the scrollback and in view after a motion.
func (c *CopyMode) follow(src ScreenSource, sc ScreenInfo) {
	if sc.mouse {
		c.wheelToCursor(src, sc)
		return
	}
	c.Cursor.Line = max(sc.base, min(c.Cursor.Line, sc.last()))
	c.scrollToCursor(src, sc)
}

func (c *CopyMode) toggleSelect(linewise bool) {
	if c.Selecting && c.linewise == linewise {
		c.Selecting = false
		return
	}
	if !c.Selecting {
		c.anchor = c.Cursor
	}
	c.Selecting, c.linewise = true, linewise
}

// exitKey handles the keys that copy or leave; other keys do nothing.
func (c *CopyMode) exitKey(k string, src ScreenSource) CopyResult {
	switch k {
	case "y", "enter":
		c.copySelection(src)
		return c.leave(src)
	case "q":
		return c.leave(src)
	case "esc":
		if c.Selecting {
			c.Selecting = false
			return CopyResult{}
		}
		return c.leave(src)
	}
	return CopyResult{}
}

// wheelToCursor keeps the cursor on screen for a child that reports the
// mouse, such as claude in its fullscreen mode: its history lives in the
// child, not the holder's scrollback, so a move past an edge turns the
// child's wheel, one tick per line. A selection pins the child, since the
// text under it would move. Ticks go wheelTickGap apart, as claude speeds up
// a burst of them.
func (c *CopyMode) wheelToCursor(src ScreenSource, sc ScreenInfo) {
	top, h := sc.top, sc.height
	over := 0
	if c.Cursor.Line < top {
		over = c.Cursor.Line - top
	} else if bottom := top + h - 1; c.Cursor.Line > bottom {
		over = c.Cursor.Line - bottom
	}
	c.Cursor.Line -= over
	if over == 0 || c.Selecting {
		return
	}
	x, y, down := c.Cursor.Col, c.Cursor.Line-top, over > 0
	for i := range max(over, -over) {
		src.write(func(s AttachStream) error {
			if i > 0 {
				time.Sleep(wheelTickGap)
			}
			return s.Wheel(x, y, down)
		})
	}
}

// wheelTickGap is the least gap at which claude scrolls a line per tick.
const wheelTickGap = 40 * time.Millisecond

// scrollToCursor scrolls the view through the holder just enough to show
// the cursor.
func (c *CopyMode) scrollToCursor(src ScreenSource, sc ScreenInfo) {
	line, top, h := c.Cursor.Line, sc.top, sc.height
	offset := sc.offset
	if line < top {
		offset = sc.base + sc.scrollMax - line
	} else if line >= top+h {
		offset = sc.base + sc.scrollMax - (line - h + 1)
	}
	if offset = max(0, min(offset, sc.scrollMax)); offset != sc.offset {
		src.scroll(offset)
	}
}

// selection is the selected range, end exclusive as the holder takes it.
func (c *CopyMode) selection() (start, end holder.LinePos) {
	start, end = c.anchor, c.Cursor
	if end.Line < start.Line || (end.Line == start.Line && end.Col < start.Col) {
		start, end = end, start
	}
	if c.linewise {
		// The end column reaches past any line; the holder clamps it.
		return holder.LinePos{Line: start.Line}, holder.LinePos{Line: end.Line, Col: 1 << 30}
	}
	end.Col++
	return start, end
}

// copySelection asks the holder for the selected text; its reply goes to
// the clipboard in updateAttachStream. Without a selection nothing is copied.
func (c *CopyMode) copySelection(src ScreenSource) {
	if c.Selecting {
		start, end := c.selection()
		src.write(func(s AttachStream) error { return s.Copy(start, end) })
	}
}

func (c *CopyMode) leave(src ScreenSource) CopyResult {
	src.snapToBottom()
	return CopyResult{Done: true}
}

// WriteCopied puts the holder's copy reply on the clipboard. The reply can
// land after copy mode has ended, so this is not a copyMode method.
func WriteCopied(clipWrite func(string) error, text string) tea.Cmd {
	if err := clipWrite(text); err != nil {
		return notify.Error("copy failed: " + err.Error())
	}
	return notify.Success("copied")
}

// CopyRows draws the cursor and the selection over the rows in view.
func (c *CopyMode) CopyRows(rows []string, sc ScreenInfo) []string {
	top, cols := sc.top, sc.cols
	var start, end holder.LinePos
	if c.Selecting {
		start, end = c.selection()
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		line := top + i
		if c.Selecting && line >= start.Line && line <= end.Line {
			from, to := 0, cols
			if line == start.Line {
				from = start.Col
			}
			if line == end.Line {
				to = min(end.Col, cols)
			}
			row = overlayCells(row, from, to, StyleCopySelection)
		}
		if line == c.Cursor.Line {
			row = overlayCells(row, c.Cursor.Col, c.Cursor.Col+1, styleCopyCursor)
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
func wordMove(k string, pos holder.LinePos, first, last int, text func(int) string) holder.LinePos {
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
	return holder.LinePos{Line: first + cur.line, Col: cur.col}
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
