package status

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/lipgloss/v2"
)

func (m Model) errorModalView() string {
	return components.RenderOutputModal(
		"Error",
		m.errorVP.View(),
		ui.HintDismissAndScroll(),
		ui.ColorRed,
		ui.ColorRed,
		ui.ColorSubtle,
		m.errorVP.Width(),
	)
}

func (m Model) helpModalView() string {
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "Keybindings",
		TitleInBorder: true,
		Body:          m.helpVP.View(),
		Hint:          ui.JoinStatus(ui.RenderInlineBindings(stageKeyHelp), ui.HintDismissAndScroll()),
		Width:         m.helpVP.Width(),
		BorderColor:   ui.ColorBlue,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorSubtle,
	})
}

func (m Model) panelStyle(active bool) lipgloss.Style {
	borderColor := ui.ColorSubtle
	if active {
		borderColor = ui.ColorOrange
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(ui.ColorBase)
}

func (m Model) renderPanelWithBorderTitle(width, height int, title, rightTitle string, lines []string, active bool, section diffSection) string {
	borderColor := ui.ColorSubtle
	titleColor := ui.ColorBlue
	if section == sectionStaged {
		borderColor = ui.ColorGreen
		titleColor = ui.ColorGreen
	} else if active {
		borderColor = ui.ColorOrange
		titleColor = ui.ColorOrange
	}
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       width,
		Height:      height,
		Title:       title,
		RightTitle:  rightTitle,
		Lines:       lines,
		BorderColor: borderColor,
		TitleColor:  titleColor,
		Background:  ui.ColorBase,
	})
}

type statusPaneIcons struct {
	folderClosed string
	folderOpen   string
	fileModified string
	fileNew      string
	fileDeleted  string
	fileRenamed  string
	fileSymlink  string
	partial      string
	staged       string
}

func statusPaneIconsFor(useNerdFontIcons bool) statusPaneIcons {
	shared := ui.Icons(useNerdFontIcons)
	return statusPaneIcons{
		folderClosed: shared.FolderClosed,
		folderOpen:   shared.FolderOpen,
		fileModified: shared.FileModified,
		fileNew:      shared.FileAdded,
		fileDeleted:  shared.FileDeleted,
		fileRenamed:  shared.FileRenamed,
		fileSymlink:  shared.FileSymlink,
		partial:      shared.Partial,
		staged:       shared.Staged,
	}
}

func statusEntryColor(entry statusEntry) string {
	if entry.Kind == statusEntryFile && isDeletedFileStatus(entry.File) {
		return "#a6adc8"
	}
	if entry.Kind == statusEntryFile && entry.File.IsRenamed() {
		return "#89b4fa"
	}
	if entry.HasStaged && entry.HasUnstaged {
		return "#fab387"
	}
	if entry.HasStaged {
		return "#a6e3a1"
	}
	return "#cdd6f4"
}

func statusEntryMeta(entry statusEntry, useNerdFontIcons bool, icons statusPaneIcons) string {
	if entry.HasStaged && entry.HasUnstaged {
		return icons.partial
	}
	if entry.HasStaged {
		return icons.staged
	}
	if useNerdFontIcons {
		return "  "
	}
	if entry.Kind == statusEntryDir {
		return "-"
	}
	return entry.File.XY()
}

func statusFileIcon(file git.StageFileStatus, isSymlink bool, icons statusPaneIcons) string {
	if isDeletedFileStatus(file) {
		return icons.fileDeleted
	}
	if file.IsRenamed() {
		return icons.fileRenamed
	}
	if isSymlink {
		return icons.fileSymlink
	}
	if file.IsUntracked() || file.IndexStatus == 'A' {
		return icons.fileNew
	}
	return icons.fileModified
}

func isDeletedFileStatus(file git.StageFileStatus) bool {
	return file.IndexStatus == 'D' || file.WorktreeCode == 'D'
}

func (m Model) flashMarker(section diffSection, rawIdx int, sec *sectionState) bool {
	if !m.flash.active || m.flash.section != section {
		return false
	}
	if m.flash.navMode == navHunk {
		if m.flash.hunk < 0 || m.flash.hunk >= len(sec.parsed.Hunks) {
			return false
		}
		h := sec.parsed.Hunks[m.flash.hunk]
		return rawIdx >= h.StartLine && rawIdx <= h.EndLine
	}
	if m.flash.line < 0 || m.flash.line >= len(sec.parsed.Changed) {
		return false
	}
	return sec.parsed.Changed[m.flash.line].LineIndex == rawIdx
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
