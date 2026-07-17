package stashlist

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
)

var (
	stashRefStyle  = lipgloss.NewStyle().Foreground(ui.ColorTeal)
	stashMsgStyle  = lipgloss.NewStyle().Foreground(ui.ColorText)
	stashTimeStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)
)

func (m listPanel) View() tea.View {
	lines := m.visibleLines()
	height := max(1, m.height-1)
	active := !m.inactive
	accent := color.Color(nil)
	if active {
		accent = m.frameTitleColor()
	}
	return tea.NewView(ui.RenderPanel(ui.PanelOptionsFor(
		m.width, height, "Stash", "", lines, active, m.frameTitleColor(), accent, m.sidebarMode,
	)))
}

func (m listPanel) frameTitleColor() color.Color {
	if m.isContainerFocused() {
		return ui.ColorOrange
	}
	return ui.ColorMauve
}

func (m listPanel) frameBorderColor() color.Color {
	if m.isContainerFocused() {
		return ui.ColorOrange
	}
	return ui.ColorBorder
}

func (m listPanel) isContainerFocused() bool {
	return !m.inactive
}

func (m listPanel) visibleLines() []string {
	if !m.loaded {
		return []string{ui.StyleMuted.Render("loading…")}
	}
	if m.err != nil {
		return []string{ui.StyleWarning.Render("error: " + m.err.Error())}
	}
	if len(m.entries) == 0 {
		return []string{ui.StyleMuted.Render("no stashes")}
	}

	innerW := max(1, m.width-4)
	start, end := m.list.VisibleRange(len(m.entries), m.visibleH())
	sel := m.list.Selected()

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.entries[i], i == sel, innerW))
	}
	return lines
}

func (m listPanel) renderRow(entry git.StashEntry, selected bool, width int) string {
	ref := stashRefStyle.Render(entry.Ref)
	ts := stashTimeStyle.Render(ui.RelativeTimeCompact(entry.Timestamp))
	msg := stashMsgStyle.Render(entry.Message)

	tsW := ansi.StringWidth(ts)
	refW := ansi.StringWidth(ref)
	gap := 1
	msgW := max(1, width-refW-gap-tsW-gap)
	msg = ansi.Truncate(msg, msgW, "…")
	actualMsgW := ansi.StringWidth(msg)
	if actualMsgW < msgW {
		msg += strings.Repeat(" ", msgW-actualMsgW)
	}

	line := fmt.Sprintf("%s %s %s", ref, msg, ts)
	lineW := ansi.StringWidth(line)
	if lineW < width {
		line += strings.Repeat(" ", width-lineW)
	}

	if selected {
		return ui.RenderRowHighlight(line)
	}
	return line
}
