package log

import (
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/splitview"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const logFlashDuration = 2 * time.Second

var logFlashBg = lipgloss.Color("#3d2810")

var (
	logHashStyle       = lipgloss.NewStyle().Foreground(ui.ColorBlue)
	logMetaStyle       = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)
	logPseudoStyle     = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
	logSearchStyle     = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true).Underline(true)
	logPushedStyle     = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	logUnpushedStyle   = lipgloss.NewStyle().Foreground(ui.ColorOrange)
	logDivergedStyle   = lipgloss.NewStyle().Foreground(ui.ColorRed)
	logRemoteOnlyStyle = lipgloss.NewStyle().Foreground(ui.ColorMauve)
)

func (m Model) View() tea.View {
	if !m.ready || m.rows == nil {
		return ui.NewMainView("\n  Loading log…")
	}
	if m.err != nil {
		return ui.NewMainView("\n  Error: " + m.err.Error())
	}

	// When detail panel is fullscreened, render it exclusively.
	if m.split.IsFullscreen() && m.split.IsDetailFocused() {
		return m.commitDetail.View()
	}

	title := "Log"
	if m.worktreeRoot != "" {
		title = fmt.Sprintf("Log (%s)", m.worktreeRoot)
	}

	lw := maxInt(20, m.listWidth())
	lh := maxInt(4, m.listHeight()-1)

	body := ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       lw,
		Height:      lh,
		Title:       title,
		RightTitle:  m.frameRightTitle(),
		Lines:       m.visibleLines(),
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		Background:  ui.ColorBase,
	})
	listOut := body
	if m.search.Mode() == search.SearchModeInput {
		overlayW := m.searchOverlayWidth()
		m.search.SetWidth(overlayW)
		overlay := m.search.View()
		y := m.settings.InputModalBottom.ResolveY(m.listHeight(), lipgloss.Height(overlay))
		listOut = ui.OverlayBottomCenter(listOut, overlay, m.listWidth(), y)
	}
	if prefix := m.keys.Prefix(); len(prefix) > 0 {
		hints := ui.ChordBindingsFromHints(m.keys.ChordHints())
		if len(hints) > 0 {
			listOut = ui.OverlayBottomRight(listOut, ui.RenderChordOverlay(prefix[0], hints), m.listWidth(), m.listHeight())
		}
	}

	// Compose with detail panel when in split mode.
	var out string
	if m.split.IsSplit() {
		detailContent := m.commitDetail.View().Content
		if m.split.EffectiveOrientation() == splitview.Vertical {
			out = lipgloss.JoinHorizontal(lipgloss.Top, listOut, detailContent)
		} else {
			out = lipgloss.JoinVertical(lipgloss.Left, listOut, detailContent)
		}
	} else {
		out = listOut
	}

	if m.rebaseConfirm.isOpen() {
		out = ui.OverlayCenter(out, m.rebaseConfirmView(m.width), m.width, m.height)
	}
	if m.amendConfirm.IsOpen {
		out = ui.OverlayCenter(out, m.amendConfirm.View(m.width), m.width, m.height)
	}
	if m.bump.IsOpen {
		out = ui.OverlayCenter(out, m.bump.View(m.width), m.width, m.height)
	}
	if m.push.IsOpen {
		out = ui.OverlayCenter(out, m.push.View(m.width), m.width, m.height)
	}
	if m.pull.IsOpen {
		out = ui.OverlayCenter(out, m.pull.View(m.width), m.width, m.height)
	}
	if m.output.IsOpen {
		out = ui.OverlayCenter(out, m.output.View(), m.width, m.height)
	}
	if m.reword.IsOpen {
		out = ui.OverlayCenter(out, m.reword.View(m.width), m.width, m.height)
	}
	if m.help.IsOpen {
		out = ui.OverlayCenter(out, m.help.View(), m.width, m.height)
	}
	return ui.NewMainView(out)
}

func (m Model) frameRightTitle() string {
	searchStatus := m.searchMatchStatus()
	context := m.startRef
	if m.filter.IsActive() {
		if m.filter.StartLine > 0 {
			context = fmt.Sprintf("%s L%d-%d", m.filter.Path, m.filter.StartLine, m.filter.EndLine)
		} else {
			context = m.filter.Path
		}
	}
	return ui.JoinStatus(context, searchStatus)
}

func (m Model) searchMatchStatus() string {
	if m.search.HasQuery() && m.search.MatchesCount() > 0 {
		return fmt.Sprintf("%d/%d matches", m.search.Cursor()+1, m.search.MatchesCount())
	}
	return ""
}

func (m Model) visibleLines() []string {
	if len(m.rows) == 0 {
		return []string{ui.StyleMuted.Render("no commits")}
	}

	innerHeight := maxInt(1, m.listHeight()-3)
	start, end := m.list.VisibleRange(len(m.rows), innerHeight)

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.rows[i], i == m.list.Selected(), m.listWidth()-4))
	}
	return lines
}

func (m Model) renderRow(row row, selected bool, width int) string {
	line := ""
	switch row.kind {
	case rowPseudoStatus:
		line = fmt.Sprintf(
			"  %s  %s",
			logPseudoStyle.Render(m.highlightSearch("working tree")),
			ui.StyleMuted.Render(m.highlightSearch(row.detail)),
		)
	default:
		line = m.renderCommitRow(row)
		if badges := m.renderBadges(row.commit.Decorations); badges != "" {
			line += "  " + badges
		}
	}
	line = ansi.Truncate(line, maxInt(1, width), "…")
	lineW := ansi.StringWidth(line)
	if lineW < width {
		line += strings.Repeat(" ", width-lineW)
	}
	if row.kind == rowCommit &&
		row.commit.Subject == m.flashSubject &&
		!m.flashUntil.IsZero() &&
		time.Now().Before(m.flashUntil) {
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

func (m Model) renderCommitRow(row row) string {
	graph := row.commit.Graph
	if graph == "" {
		graph = "*"
	}
	state := commitState(row.class, m.branchDiverged)
	cols := []ui.FixedColumn{
		{Text: graph, Width: 4},
		{Text: m.highlightSearch(row.commit.Hash), Width: 8, Style: logHashStyle},
		{Text: m.highlightSearch(row.commit.AuthorShort), Width: 3, Style: logMetaStyle},
		{Text: ui.RelativeTimeCompact(row.commit.Date), Width: 10, Style: logMetaStyle},
		{Text: state.icon, Width: 1, Style: state.style},
	}
	meta := ui.RenderFixedColumns(cols)
	return meta + " " + state.style.Render(m.highlightSearch(row.commit.Subject))
}

func (m Model) renderBadges(decorations []git.RefDecoration) string {
	if len(decorations) == 0 {
		return ""
	}
	nerd := m.settings.UseNerdFontIcons
	visible := make([]git.RefDecoration, 0, len(decorations))
	for _, dec := range decorations {
		if !isHiddenRef(dec.Name, m.compiledHideRefs) {
			visible = append(visible, dec)
		}
	}
	sorted := sortDecorations(visible, m.compiledRefRules)
	parts := make([]string, 0, len(sorted))
	for _, dec := range sorted {
		label := m.highlightSearch(dec.Name)
		if c, ok := matchRefRule(dec.Name, m.compiledRefRules); ok {
			parts = append(parts, ui.RenderBadgeWithColor(label, c, nerd, false))
		} else {
			parts = append(parts, ui.RenderBadge(label, ui.BadgeVariantDeepBg, nerd, false))
		}
	}
	return strings.Join(parts, " ")
}

func (m Model) highlightSearch(text string) string {
	query := strings.TrimSpace(m.search.Query())
	if query == "" {
		return text
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	if !strings.Contains(lowerText, lowerQuery) {
		return text
	}

	var out strings.Builder
	start := 0
	for start < len(text) {
		idx := strings.Index(lowerText[start:], lowerQuery)
		if idx < 0 {
			out.WriteString(text[start:])
			break
		}
		idx += start
		out.WriteString(text[start:idx])
		end := idx + len(query)
		out.WriteString(logSearchStyle.Render(text[idx:end]))
		start = end
	}
	return out.String()
}

func (m Model) searchOverlayWidth() int {
	outerW := m.width * 80 / 100
	if outerW > 50 {
		outerW = 50
	}
	if outerW < 20 {
		outerW = 20
	}
	return outerW
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
