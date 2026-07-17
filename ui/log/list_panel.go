package log

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/list"
)

const logFlashDuration = 2 * time.Second

var logFlashBg = lipgloss.Color("#3d2810")

var (
	logMetaStyle         = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)
	logPseudoStyle       = lipgloss.NewStyle().Foreground(ui.ColorYellow).Italic(true)
	logPseudoStatusStyle = ui.StyleMuted.Italic(true)
	logSearchStyle       = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true).Underline(true)
)

// subjectIndent lines the second (metadata) line of a commit row up under
// the subject column of the first (subject) line, when the graph column is
// shown (graph width 4 + push/pull state width 2).
const subjectIndent = "      "

// subjectIndentNoGraph is the metadata line indent when the graph column is
// hidden (push/pull state width 2 only).
const subjectIndentNoGraph = "  "

// listPanelHints carries page-owned render state into the list panel.
type listPanelHints struct {
	title            string
	rightTitle       string
	highlight        func(string) string
	flashSubject     string
	flashUntil       time.Time
	branchDiverged   bool
	compiledRefRules []compiledRefRule
	compiledHideRefs []*regexp.Regexp
	showGraph        bool
}

// listPanel is the log list panel. It implements splitview.ListPanel.
type listPanel struct {
	rows        []row
	list        list.Model
	width       int
	height      int
	inactive    bool
	sidebarMode bool
	hints       listPanelHints
}

func newListPanel() listPanel { return listPanel{} }

// WithContainerFocus returns a copy that renders as active only when focused.
func (m listPanel) WithContainerFocus(focused bool) listPanel {
	m.inactive = !focused
	return m
}

// WithSidebarMode returns a copy that renders with the sidebar-mode
// background (see CONTEXT.md) when the commit list is shown alongside the
// detail panel, as opposed to standalone.
func (m listPanel) WithSidebarMode(sidebar bool) listPanel {
	m.sidebarMode = sidebar
	return m
}

// WithRows sets the rows to display.
func (m listPanel) WithRows(rows []row) listPanel {
	m.rows = rows
	return m
}

// WithHints sets the render hints for the next render.
func (m listPanel) WithHints(h listPanelHints) listPanel {
	m.hints = h
	return m
}

// Rows returns the current rows slice (for page-level search/jump).
func (m listPanel) Rows() []row { return m.rows }

// Selected returns the current cursor position.
func (m listPanel) Selected() int { return m.list.Selected() }

// Navigate moves the cursor by delta rows.
func (m listPanel) Navigate(delta int) listPanel {
	m.list.Navigate(delta, len(m.rows), m.visibleH())
	return m
}

// SetSelected moves the cursor to position i and ensures it's visible.
func (m listPanel) SetSelected(i int) listPanel {
	m.list.SetSelected(i, len(m.rows))
	m.list.EnsureSelectionVisible(len(m.rows), m.visibleH())
	return m
}

// ScrollPage scrolls by delta pages.
func (m listPanel) ScrollPage(delta int) listPanel {
	m.list.ScrollPage(delta, len(m.rows), m.visibleH())
	return m
}

// ScrollViewport scrolls the viewport by delta rows without moving selection.
func (m listPanel) ScrollViewport(delta int) listPanel {
	m.list.ScrollViewport(delta, len(m.rows), m.visibleH())
	return m
}

// SelectedRef satisfies splitview.ListPanel.
func (m listPanel) SelectedRef() string {
	if len(m.rows) == 0 {
		return ""
	}
	cursor := m.list.Selected()
	if cursor < 0 || cursor >= len(m.rows) {
		return ""
	}
	if m.rows[cursor].kind != rowCommit {
		return ""
	}
	return m.rows[cursor].commit.FullHash
}

func (m listPanel) Init() tea.Cmd { return nil }

func (m listPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
	}
	return m, nil
}

// visibleH returns how many rows fit on screen. Commit rows render as two
// physical lines, so this is conservative: it assumes every row takes two
// lines, which slightly under-counts when the single-line pseudo-status row
// is in view but never overflows the frame.
func (m listPanel) visibleH() int {
	h := (m.height - 3) / 2
	if h < 1 {
		return 1
	}
	return h
}

// View renders the log list panel frame.
func (m listPanel) View() tea.View {
	lw := maxInt(20, m.width)
	lh := maxInt(4, m.height-1)
	active := !m.inactive
	accent := color.Color(nil)
	if active {
		accent = m.frameTitleColor()
	}
	return tea.NewView(ui.RenderPanel(ui.PanelOptionsFor(
		lw, lh, m.hints.title, m.hints.rightTitle, m.visibleLines(), active, m.frameTitleColor(), accent, m.sidebarMode,
	)))
}

func (m listPanel) frameTitleColor() color.Color {
	if !m.inactive {
		return ui.ColorOrange
	}
	return ui.ColorBlue
}

func (m listPanel) frameBorderColor() color.Color {
	if !m.inactive {
		return ui.ColorOrange
	}
	return ui.ColorBorder
}

func (m listPanel) visibleLines() []string {
	if len(m.rows) == 0 {
		return []string{ui.StyleMuted.Render("no commits")}
	}
	rowBudget := maxInt(1, m.visibleH())
	start, end := m.list.VisibleRange(len(m.rows), rowBudget)
	lines := make([]string, 0, (end-start)*2)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.rows[i], i == m.list.Selected(), m.width-2)...)
	}
	return lines
}

// renderRow renders one row as one or more physical lines (pseudo-status
// rows are a single line; commit rows are a subject line plus an indented
// metadata line), each padded/truncated to width and background-styled
// uniformly so a selection or flash highlight covers the full row.
func (m listPanel) renderRow(r row, selected bool, width int) []string {
	var rawLines []string
	switch r.kind {
	case rowPseudoStatus:
		rawLines = []string{fmt.Sprintf(
			"%s: %s",
			logPseudoStyle.Render(m.hl("working tree")),
			logPseudoStatusStyle.Render(m.hl(r.detail)),
		)}
	default:
		rawLines = m.renderCommitRow(r)
	}

	flash := r.kind == rowCommit &&
		r.commit.Subject == m.hints.flashSubject &&
		!m.hints.flashUntil.IsZero() &&
		time.Now().Before(m.hints.flashUntil)

	lines := make([]string, len(rawLines))
	for i, line := range rawLines {
		line = ansi.Truncate(line, maxInt(1, width), "…")
		lineW := ansi.StringWidth(line)
		if lineW < width {
			line += strings.Repeat(" ", width-lineW)
		}
		switch {
		case flash:
			line = ui.RenderRowWithBackground(line, logFlashBg)
		case selected:
			line = ui.RenderRowHighlight(line)
		}
		lines[i] = line
	}
	return lines
}

// renderCommitRow renders a commit as two lines: the subject line up top
// (graph, push/pull state, subject) and an indented metadata line below it
// (hash, date, author, decoration badges).
func (m listPanel) renderCommitRow(r row) []string {
	state := ui.CommitPushState(r.class, m.hints.branchDiverged)
	date := ui.RelativeTimeCompact(r.commit.Date)
	cols := make([]ui.FixedColumn, 0, 2)
	if m.hints.showGraph {
		graph := r.commit.Graph
		if graph == "" {
			graph = "*"
		}
		cols = append(cols, ui.FixedColumn{Text: graph, Width: 4})
	}
	cols = append(cols, ui.FixedColumn{Text: state.Icon, Width: 2, Style: state.Style})
	subject := ui.RenderFixedColumns(cols) + state.Style.Render(m.hl(r.commit.Subject))

	indent := subjectIndentNoGraph
	if m.hints.showGraph {
		indent = subjectIndent
	}
	meta := indent + logMetaStyle.Render(m.hl(r.commit.Hash)) + " " +
		logMetaStyle.Render(date) + logMetaStyle.Render(" by ") + logMetaStyle.Render(m.hl(r.commit.AuthorShort))
	if badges := m.renderBadges(r.commit.Decorations); badges != "" {
		meta += logMetaStyle.Render(" · ") + badges
	}
	return []string{subject, meta}
}

// renderBadges renders decoration names as plain colored text (no pill
// background), separated from the rest of the metadata line by a dot.
func (m listPanel) renderBadges(decorations []git.RefDecoration) string {
	if len(decorations) == 0 {
		return ""
	}
	visible := make([]git.RefDecoration, 0, len(decorations))
	for _, dec := range decorations {
		if !isHiddenRef(dec.Name, m.hints.compiledHideRefs) {
			visible = append(visible, dec)
		}
	}
	sorted := sortDecorations(visible, m.hints.compiledRefRules)

	parts := make([]string, 0, len(sorted))
	for _, dec := range sorted {
		parts = append(parts, ui.RenderBadgeText(m.hl(dec.Name), m.decorationColor(dec)))
	}
	return strings.Join(parts, " ")
}

// decorationColor returns the foreground color a decoration would use as its
// own separate badge, matching ui.BadgeVariantDeepBg's foreground when no
// rule matches.
func (m listPanel) decorationColor(dec git.RefDecoration) color.Color {
	if c, ok := matchRefRule(dec.Name, m.hints.compiledRefRules); ok {
		return c
	}
	return ui.ColorSubtle
}

// hl applies the search-highlight function to text, falling back to identity.
func (m listPanel) hl(text string) string {
	if m.hints.highlight == nil {
		return text
	}
	return m.hints.highlight(text)
}
