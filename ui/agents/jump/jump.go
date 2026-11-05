// Package jump is the jump-to-agent popup of the agents tab.
package jump

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/agents/agentstyle"
	"github.com/elentok/gx/ui/fuzzy"
)

const maxWidth = 100

var hint = ui.RenderInlineBindings(
	key.NewBinding(key.WithHelp("ctrl+n/ctrl+p or ↑/↓", "navigate")),
	key.NewBinding(key.WithHelp("type", "filter")),
	key.NewBinding(key.WithHelp("tab", "order")),
	key.NewBinding(key.WithHelp("enter", "jump")),
	key.NewBinding(key.WithHelp("esc", "close")),
)

// Popup is the jump-to-agent popup; cursor indexes its matches.
type Popup struct {
	query        string
	projectOrder bool
	cursor       int
}

// Ctx is what the popup reads from the agents Model: the sessions it lists and
// what it needs to draw them.
type Ctx struct {
	Sessions    []wire.SessionInfo
	Now         func() time.Time
	Width       int
	Height      int
	Settings    ui.Settings
	SpinnerView string
}

// Result is what a key did beyond editing the popup: Closed when it is
// gone, Picked when Session is the index into Ctx.Sessions to jump to.
type Result struct {
	Closed  bool
	Picked  bool
	Session int
}

func (j *Popup) attention() bool { return !j.projectOrder }

// Matches is the indexes into c.Sessions the popup lists: filtered, and in
// attention order (Actionable, Unread, the rest) unless toggled to the
// sidebar's project order.
func (j *Popup) Matches(c Ctx) []int {
	var out []int
	for i, s := range c.Sessions {
		if fuzzy.Match(j.query, agentstyle.DisplayName(s), s.Project, s.Dir) {
			out = append(out, i)
		}
	}
	if j.attention() {
		slices.SortStableFunc(out, func(a, b int) int {
			return attentionBucket(c.Sessions[a]) - attentionBucket(c.Sessions[b])
		})
	}
	return out
}

func attentionBucket(s wire.SessionInfo) int {
	switch {
	case agentstyle.IsActionable(s):
		return 0
	case s.Unread.Unread:
		return 1
	}
	return 2
}

// Update applies a key and returns the new popup state, never mutating j.
func (j *Popup) Update(msg tea.KeyPressMsg, c Ctx) (*Popup, Result) {
	next := *j
	matches := next.Matches(c)
	switch msg.String() {
	case "ctrl+n", "down":
		next.cursor = min(next.cursor+1, max(0, len(matches)-1))
	case "ctrl+p", "up":
		next.cursor = max(next.cursor-1, 0)
	case "tab":
		next.projectOrder = !next.projectOrder
		next.cursor = 0
	case "esc":
		return nil, Result{Closed: true}
	case "enter":
		if next.cursor >= len(matches) {
			return &next, Result{}
		}
		return nil, Result{Closed: true, Picked: true, Session: matches[next.cursor]}
	case "backspace":
		if r := []rune(next.query); len(r) > 0 {
			next.query = string(r[:len(r)-1])
			next.cursor = 0
		}
	default:
		if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			next.query += msg.Text
			next.cursor = 0
		}
	}
	return &next, Result{}
}

// bodyHeight is the popup's room for match rows: the screen less the
// border, query line, the gaps and the hint.
func (j *Popup) bodyHeight(c Ctx) int { return max(1, c.Height-7) }

// offset is the first match on screen, scrolled to keep the cursor shown.
func (j *Popup) offset(c Ctx) int { return max(0, j.cursor-j.bodyHeight(c)+1) }

// Shown is the matches on screen in the popup.
func (j *Popup) Shown(c Ctx) []int {
	matches := j.Matches(c)
	offset := min(j.offset(c), len(matches))
	return matches[offset:min(len(matches), offset+j.bodyHeight(c))]
}

func (j *Popup) Render(c Ctx) string {
	width := min(maxWidth, c.Width-4)
	innerW := width - 4
	matches := j.Matches(c)
	order := "attention"
	if !j.attention() {
		order = "project"
	}
	lines := []string{ui.StyleMuted.Render("> ") + j.query}
	if len(matches) == 0 {
		lines = append(lines, ui.StyleMuted.Render("  no matches"))
	}
	offset := j.offset(c)
	for n, i := range j.Shown(c) {
		row := j.row(c, c.Sessions[i], innerW)
		if offset+n == j.cursor {
			row = ui.RenderRowHighlight(row)
		}
		lines = append(lines, row)
	}
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "Jump to agent",
		RightTitle:    order + " · " + strconv.Itoa(len(matches)) + "/" + strconv.Itoa(len(c.Sessions)),
		Body:          strings.Join(lines, "\n"),
		Hint:          hint,
		Width:         width,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorSubtle,
		TitleInBorder: true,
	})
}

// row is status icon, marks, name, project, dir, the time since the last
// status change, status label and the Actionable reason or status detail.
func (j *Popup) row(c Ctx, s wire.SessionInfo, w int) string {
	icons := ui.Icons(c.Settings.UseNerdFontIcons)
	style := agentstyle.NameRead
	if s.Unread.Unread {
		style = agentstyle.NameUnread
	}
	since := ""
	if !s.StatusChangedAt.IsZero() {
		since = ui.DurationCompact(c.Now().Sub(s.StatusChangedAt))
	}
	// Some status glyphs render 2 cells wide; a fixed slot keeps columns aligned.
	row := agentstyle.Fit(agentstyle.StatusIcon(s, icons, c.SpinnerView), 2) + " " + agentstyle.Marks(s, icons) +
		agentstyle.Fit(style.Render(agentstyle.DisplayName(s)), 18) + " " +
		agentstyle.Fit(agentstyle.ProjectHeader.Render(s.Project), 12) + " " +
		agentstyle.Fit(ui.StyleMuted.Render(agentstyle.ShortDir(c.Settings.Home(), s.Dir)), 20) + " " +
		agentstyle.Fit(ui.StyleMuted.Render(since), 5) + " " +
		agentstyle.StatusLabel(s)
	if s.Actionable.Reason != "" {
		row += " " + agentstyle.Reason.Render(s.Actionable.Reason)
	} else if d := agentstyle.StatusDetail(s); d != "" {
		row += " " + agentstyle.Detail.Render(d)
	}
	return agentstyle.Fit(row, w)
}
