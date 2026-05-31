package status

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/status/diffarea"
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

func (m Model) renderFiletreePanelWithBorderTitle(width, height int, title, rightTitle string, lines []string, active bool) string {
	borderColor := ui.ColorBorder
	titleColor := ui.ColorBlue
	if active {
		borderColor = ui.ColorBlue
		titleColor = ui.ColorBlue
	}
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       width,
		Height:      height,
		Title:       title,
		RightTitle:  rightTitle,
		Lines:       lines,
		BorderColor: borderColor,
		TitleColor:  titleColor,
		TitleBold:   active,
		Background:  ui.ColorBase,
	})
}

func (m Model) renderPanelWithBorderTitle(width, height int, title, rightTitle string, lines []string, active bool, section diffarea.Section) string {
	highlightMoved := m.diffarea.Flash.Active && m.diffarea.Flash.Section == section
	borderColor := ui.ColorBorder
	titleColor := ui.ColorOrange
	if section == diffarea.SectionStaged {
		borderColor = ui.ColorGreen
		titleColor = ui.ColorGreen
	} else {
		borderColor = ui.ColorOrange
		titleColor = ui.ColorOrange
	}
	if !active {
		borderColor = ui.ColorBorder
		if section == diffarea.SectionStaged {
			titleColor = ui.ColorGreen
		} else {
			titleColor = ui.ColorOrange
		}
	} else if section == diffarea.SectionStaged {
		borderColor = ui.ColorGreen
		titleColor = ui.ColorGreen
	} else {
		borderColor = ui.ColorOrange
		titleColor = ui.ColorOrange
	}
	if highlightMoved {
		borderColor = titleColor
	}
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       width,
		Height:      height,
		Title:       title,
		RightTitle:  rightTitle,
		Lines:       lines,
		BorderColor: borderColor,
		TitleColor:  titleColor,
		TitleBold:   active || highlightMoved,
		Background:  ui.ColorBase,
	})
}

type filetreePaneIcons struct {
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

func filetreePaneIconsFor(useNerdFontIcons bool) filetreePaneIcons {
	shared := ui.Icons(useNerdFontIcons)
	return filetreePaneIcons{
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

func statusEntryMeta(entry statusEntry, useNerdFontIcons bool, icons filetreePaneIcons) string {
	if entry.HasStaged && entry.HasUnstaged {
		return icons.partial
	}
	if entry.HasStaged {
		return icons.staged
	}
	if useNerdFontIcons {
		return " "
	}
	if entry.Kind == statusEntryDir {
		return "-"
	}
	return entry.File.XY()
}

func statusFileIcon(file git.StageFileStatus, isSymlink bool, icons filetreePaneIcons) string {
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
