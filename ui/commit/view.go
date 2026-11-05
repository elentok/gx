package commit

import (
	"image/color"
	"regexp"
	"strings"

	"github.com/elentok/gx/git"

	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/refrules"
	"github.com/elentok/gx/ui/tree"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	FILE_TREE_MIN_WIDTH = 25
	FILE_TREE_MAX_WIDTH = 45
	DIFF_MIN_WIDTH      = 40
)

var commitMetaStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)
var commitSubjectStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)

func (m Model) View() tea.View {
	if !m.ready {
		return ui.NewMainView("\n  Loading commit…")
	}
	if m.err != nil {
		return ui.NewMainView("\n  Error: " + m.err.Error())
	}

	out := m.contentView(m.layout())
	if prefix := m.keys.Prefix(); len(prefix) > 0 {
		hints := ui.ChordBindingsFromHints(m.keys.ChordHints())
		if len(hints) > 0 {
			out = ui.OverlayBottomRight(out, ui.RenderChordOverlay(prefix[0], hints), m.width, m.height)
		}
	}
	out = m.commitOps.Overlay(out, m.width, m.height)
	if m.help.IsOpen {
		out = ui.OverlayCenter(out, m.help.View(), m.width, m.height)
	}
	if m.focusHeader {
		out = ui.OverlayCenter(out, m.header.renderHeaderPopup(m.headerInput()), m.width, m.height)
	}
	return ui.NewMainView(out)
}

// metaLineWithBadges appends as many badges as fit onto metaLine (separated
// from the meta text by a dot), then packs any remaining badges onto further
// lines. With no badges, metaLine is the sole line.
func metaLineWithBadges(metaLine string, badges []string, maxWidth int) []string {
	if len(badges) == 0 {
		return []string{metaLine}
	}
	sep := commitMetaStyle.Render(" · ")
	lines := []string{metaLine}
	avail := maxWidth - ansi.StringWidth(metaLine)
	i := 0
	for i < len(badges) {
		prefix := " "
		if i == 0 {
			prefix = sep
		}
		badgeW := ansi.StringWidth(badges[i])
		if ansi.StringWidth(prefix)+badgeW > avail {
			break
		}
		lines[0] += prefix + badges[i]
		avail -= ansi.StringWidth(prefix) + badgeW
		i++
	}
	if i < len(badges) {
		lines = append(lines, packBadgeLines(badges[i:], maxWidth)...)
	}
	return lines
}

// packBadgeLines greedily packs badges onto full-width lines.
func packBadgeLines(badges []string, maxWidth int) []string {
	var lines []string
	current := ""
	currentW := 0
	for _, badge := range badges {
		badgeW := ansi.StringWidth(badge)
		if current == "" {
			current = badge
			currentW = badgeW
			continue
		}
		if currentW+1+badgeW > maxWidth {
			lines = append(lines, current)
			current = badge
			currentW = badgeW
			continue
		}
		current += " " + badge
		currentW += 1 + badgeW
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func normalizeCommitBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.TrimSpace(body)
}

func (m Model) contentView(l layout) string {
	if len(m.sidebar.fileTreeModel.Entries()) == 0 {
		width := max(20, m.width)
		lines := []string{ui.StyleMuted.Render("no changed files")}
		return ui.RenderPanel(ui.PanelOptionsFor(width, l.contentH, "Changes", "", lines, false, ui.ColorBlue, nil, false))
	}

	if l.narrow() {
		filesH, diffH := l.narrowHeights()
		files := m.sidebar.renderFilesPane(m.filesInput(), m.width, filesH)
		diff := m.renderDiffPane(m.width, diffH)
		return lipgloss.JoinVertical(lipgloss.Left, files, ui.RenderSeamRow(m.width, ui.SeamColor), diff)
	}
	mainH := l.contentH
	rightW := m.width - 1 - l.filesW
	left := m.sidebar.renderFilesPane(m.filesInput(), l.filesW, mainH)
	right := m.renderDiffPane(rightW, mainH)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, ui.RenderSeamColumn(mainH, ui.SeamColor), right)
}

func (m Model) renderDiffPane(width, height int) string {
	lines := []string{ui.StyleMuted.Render("no diff")}
	if len(m.diff.diffModel.Data().ViewLines) > 0 {
		bodyH := max(1, height-2)
		innerW := max(1, width-2)
		lines = m.diff.diffModel.RenderRows(bodyH, m.focusDiff, diffview.RenderOpts{
			AccentColor: ui.ColorOrange,
			InnerWidth:  innerW,
			SearchMatch: func(displayIdx int) (bool, bool) {
				return m.diff.diffModel.SearchMatchAt(displayIdx)
			},
			SearchQuery: m.diff.diffModel.Search().Query(),
		})
	} else if len(m.diff.diffModel.Data().Parsed.Lines) > 0 {
		if diffcore.HasBinaryDiff(m.diff.diffModel.Data().Parsed) {
			file, hasFile := m.sidebar.selectedCommitFile()
			lines = m.diff.binaryDiffLines(m.settings.ImageDiffs, file, hasFile, max(1, height-2), max(1, width-2))
		} else {
			lines = []string{ui.StyleMuted.Render("no diff")}
		}
	}
	title, rightTitle := "Diff", m.diff.title(m.focusDiff, m.settings.UseNerdFontIcons)
	active := !m.inactive && m.focusDiff
	titleColor := diffPaneTitleColor(active)
	accent := color.Color(nil)
	if active {
		accent = titleColor
	}
	return ui.RenderPanel(ui.PanelOptionsFor(width, height, title, rightTitle, lines, active, titleColor, accent, false))
}

func renderBadges(decorations []git.RefDecoration) string {
	return strings.Join(decorationBadgeParts(decorations, nil, nil, ""), " ")
}

// decorationBadgeParts filters out hidden refs and sorts decorations the same
// way ui/log does (important-refs rules first, in rule order), so the tags
// shown here appear in the same order as the log view's rows.
func decorationBadgeParts(decorations []git.RefDecoration, hideRefs []*regexp.Regexp, rules []refrules.CompiledRefRule, trunk string) []string {
	if len(decorations) == 0 {
		return nil
	}
	visible := make([]git.RefDecoration, 0, len(decorations))
	for _, decoration := range decorations {
		if !refrules.IsHiddenRef(decoration.Name, hideRefs) {
			visible = append(visible, decoration)
		}
	}
	sorted := refrules.SortDecorations(visible, rules)
	parts := make([]string, 0, len(sorted))
	for _, decoration := range sorted {
		parts = append(parts, ui.RenderBadgeText(decoration.Name, badgeColorForDecoration(decoration, trunk)))
	}
	return parts
}

func badgeColorForDecoration(decoration git.RefDecoration, trunk string) color.Color {
	switch decoration.Kind {
	case git.RefDecorationTag:
		return ui.ColorBlue
	case git.RefDecorationRemoteBranch, git.RefDecorationLocalBranch:
		if isTrunkRef(decoration, trunk) {
			return ui.ColorYellow
		}
		return ui.ColorMauve
	default:
		return ui.ColorSubtle
	}
}

// isTrunkRef strips only the remote name from a remote branch, so a trunk
// with a slash ("release/v2") still matches and "feature/main" never does.
func isTrunkRef(decoration git.RefDecoration, trunk string) bool {
	if trunk == "" {
		return false
	}
	name := strings.TrimSpace(decoration.Name)
	if decoration.Kind == git.RefDecorationRemoteBranch {
		_, name, _ = strings.Cut(name, "/")
	}
	return name == trunk
}

func diffPaneTitleColor(active bool) color.Color {
	if active {
		return ui.ColorOrange
	}
	return ui.ColorBlue
}

func diffPaneBorderColor(active bool) color.Color {
	if active {
		return ui.ColorOrange
	}
	return ui.ColorBorder
}

func commitEntryMeta(entry tree.Entry[stage.CommitFile], useNerdFontIcons bool) string {
	if useNerdFontIcons {
		return "  "
	}
	if entry.HasChildren {
		return "-"
	}
	status := strings.TrimSpace(entry.Value.Status)
	if status == "" {
		return "  "
	}
	if len(status) > 2 {
		status = status[:2]
	}
	return status
}

func commitEntryColor(entry tree.Entry[stage.CommitFile]) string {
	if entry.HasChildren {
		return "#cdd6f4"
	}
	switch {
	case strings.HasPrefix(entry.Value.Status, "D"):
		return "#a6adc8"
	case strings.HasPrefix(entry.Value.Status, "R"), strings.HasPrefix(entry.Value.Status, "C"):
		return "#89b4fa"
	case strings.HasPrefix(entry.Value.Status, "A"):
		return "#a6e3a1"
	default:
		return "#cdd6f4"
	}
}

func commitFileIcon(file stage.CommitFile, useNerdFontIcons bool) string {
	icons := ui.Icons(useNerdFontIcons)
	switch {
	case strings.HasPrefix(file.Status, "D"):
		return icons.FileDeleted
	case strings.HasPrefix(file.Status, "R"), strings.HasPrefix(file.Status, "C"):
		return icons.FileRenamed
	case strings.HasPrefix(file.Status, "A"):
		return icons.FileAdded
	default:
		return icons.FileModified
	}
}
