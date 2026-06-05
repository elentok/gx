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
	logHashStyle         = lipgloss.NewStyle().Foreground(ui.ColorBlue)
	logMetaStyle         = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)
	logPseudoStyle       = lipgloss.NewStyle().Foreground(ui.ColorYellow).Italic(true)
	logPseudoStatusStyle = ui.StyleMuted.Italic(true)
	logSearchStyle       = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true).Underline(true)
	logPushedStyle       = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	logUnpushedStyle     = lipgloss.NewStyle().Foreground(ui.ColorOrange)
	logDivergedStyle     = lipgloss.NewStyle().Foreground(ui.ColorRed)
	logRemoteOnlyStyle   = lipgloss.NewStyle().Foreground(ui.ColorMauve)
)

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
	nerdFont         bool
}

// listPanel is the log list panel. It implements splitview.ListPanel.
type listPanel struct {
	rows     []row
	list     list.Model
	width    int
	height   int
	inactive bool
	hints    listPanelHints
}

func newListPanel() listPanel { return listPanel{} }

// WithContainerFocus returns a copy that renders as active only when focused.
func (m listPanel) WithContainerFocus(focused bool) listPanel {
	m.inactive = !focused
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

func (m listPanel) visibleH() int {
	h := m.height - 3
	if h < 1 {
		return 1
	}
	return h
}

// View renders the log list panel frame.
func (m listPanel) View() tea.View {
	lw := maxInt(20, m.width)
	lh := maxInt(4, m.height-1)
	return tea.NewView(ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       lw,
		Height:      lh,
		Title:       m.hints.title,
		RightTitle:  m.hints.rightTitle,
		Lines:       m.visibleLines(),
		BorderColor: m.frameBorderColor(),
		TitleColor:  m.frameTitleColor(),
		Background:  ui.ColorBase,
	}))
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
	innerHeight := maxInt(1, m.visibleH())
	start, end := m.list.VisibleRange(len(m.rows), innerHeight)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.rows[i], i == m.list.Selected(), m.width-4))
	}
	return lines
}

func (m listPanel) renderRow(r row, selected bool, width int) string {
	var line string
	switch r.kind {
	case rowPseudoStatus:
		line = fmt.Sprintf(
			"    %s           %s",
			logPseudoStyle.Render(m.hl("working tree")),
			logPseudoStatusStyle.Render(m.hl(r.detail)),
		)
	default:
		line = m.renderCommitRow(r)
		if badges := m.renderBadges(r.commit.Decorations); badges != "" {
			line += "  " + badges
		}
	}
	line = ansi.Truncate(line, maxInt(1, width), "…")
	lineW := ansi.StringWidth(line)
	if lineW < width {
		line += strings.Repeat(" ", width-lineW)
	}
	if r.kind == rowCommit &&
		r.commit.Subject == m.hints.flashSubject &&
		!m.hints.flashUntil.IsZero() &&
		time.Now().Before(m.hints.flashUntil) {
		return ui.RenderRowWithBackground(line, logFlashBg)
	}
	if selected {
		return ui.RenderRowHighlight(line)
	}
	return line
}

type commitStateInfo struct {
	icon  string
	style lipgloss.Style
}

func commitState(class git.BranchHistoryClass, branchDiverged bool) commitStateInfo {
	switch class {
	case git.BranchHistoryLocalOnly:
		if branchDiverged {
			return commitStateInfo{"󰃻", logDivergedStyle}
		}
		return commitStateInfo{"󰜷", logUnpushedStyle}
	case git.BranchHistoryShared:
		return commitStateInfo{"✔", logPushedStyle}
	case git.BranchHistoryRemoteOnly:
		return commitStateInfo{"󰜮", logRemoteOnlyStyle}
	default:
		return commitStateInfo{" ", lipgloss.NewStyle()}
	}
}

func (m listPanel) renderCommitRow(r row) string {
	graph := r.commit.Graph
	if graph == "" {
		graph = "*"
	}
	state := commitState(r.class, m.hints.branchDiverged)
	cols := []ui.FixedColumn{
		{Text: graph, Width: 4},
		{Text: m.hl(r.commit.Hash), Width: 8, Style: logHashStyle},
		{Text: m.hl(r.commit.AuthorShort), Width: 3, Style: logMetaStyle},
		{Text: ui.RelativeTimeCompact(r.commit.Date), Width: 10, Style: logMetaStyle},
		{Text: state.icon, Width: 1, Style: state.style},
	}
	meta := ui.RenderFixedColumns(cols)
	return meta + " " + state.style.Render(m.hl(r.commit.Subject))
}

func (m listPanel) renderBadges(decorations []git.RefDecoration) string {
	if len(decorations) == 0 {
		return ""
	}
	nerd := m.hints.nerdFont
	visible := make([]git.RefDecoration, 0, len(decorations))
	for _, dec := range decorations {
		if !isHiddenRef(dec.Name, m.hints.compiledHideRefs) {
			visible = append(visible, dec)
		}
	}
	sorted := sortDecorations(visible, m.hints.compiledRefRules)
	parts := make([]string, 0, len(sorted))
	for _, dec := range sorted {
		label := m.hl(dec.Name)
		if c, ok := matchRefRule(dec.Name, m.hints.compiledRefRules); ok {
			parts = append(parts, ui.RenderBadgeWithColor(label, c, nerd, false))
		} else {
			parts = append(parts, ui.RenderBadge(label, ui.BadgeVariantDeepBg, nerd, false))
		}
	}
	return strings.Join(parts, " ")
}

// hl applies the search-highlight function to text, falling back to identity.
func (m listPanel) hl(text string) string {
	if m.hints.highlight == nil {
		return text
	}
	return m.hints.highlight(text)
}
