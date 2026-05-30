package status

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filetree"

	"github.com/charmbracelet/x/ansi"
)

const (
	minFiletreePaneWidth  = 25
	maxFiletreePaneWidth  = 45
	minDiffPaneWidth      = 60
	minFiletreePaneHeight = 5
)

func (m Model) splitWidth() (filetreeW, diffW int) {
	if m.useStackedLayout() {
		return m.width, m.width
	}

	filetreeW = m.requiredFiletreePaneWidth(m.mainContentHeight())
	filetreeMax := minInt(maxFiletreePaneWidth, int(float64(m.width)*0.45))
	if filetreeMax < minFiletreePaneWidth {
		filetreeMax = minFiletreePaneWidth
	}
	if filetreeW < minFiletreePaneWidth {
		filetreeW = minFiletreePaneWidth
	}
	if filetreeW > filetreeMax {
		filetreeW = filetreeMax
	}
	if m.width-filetreeW < minDiffPaneWidth {
		filetreeW = m.width - minDiffPaneWidth
	}
	if filetreeW < minFiletreePaneWidth {
		filetreeW = minFiletreePaneWidth
	}
	diffW = m.width - filetreeW
	if diffW < minDiffPaneWidth {
		diffW = minDiffPaneWidth
		filetreeW = m.width - diffW
	}
	return filetreeW, diffW
}

func (m Model) splitHeight(total int) (filetreeH, diffH int) {
	if !m.useStackedLayout() {
		return total, total
	}
	filetreeH = int(float64(total) * 0.30)
	if filetreeH < 5 {
		filetreeH = 5
	}
	diffH = total - filetreeH
	if diffH < 5 {
		diffH = 5
		filetreeH = total - diffH
	}
	return filetreeH, diffH
}

func (m Model) useStackedLayout() bool {
	return m.width <= 100
}

func (m Model) mainContentHeight() int {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	return mainH
}

func (m Model) renderLeftPane(width, height int) string {
	return m.renderFiletreePane(width, maxInt(minFiletreePaneHeight, height))
}

func (m Model) filetreePaneTitle() string {
	title := "Filetree"
	if summary := m.branchSummaryTitleSuffix(); summary != "" {
		title += " (" + summary + ")"
	}
	return title
}

func (m Model) visibleStatusLines(width, height int) []string {
	icons := filetreePaneIconsFor(m.settings.UseNerdFontIcons)
	return m.fileTreeModel.RenderLines(height, filetree.RenderOpts[git.StageFileStatus]{
		AccentColor:      ui.ColorBlue,
		Active:           m.focus == focusFiletree,
		Width:            width - 2,
		EmptyLine:        ui.StyleMuted.Render("clean working tree"),
		UseNerdFontIcons: m.settings.UseNerdFontIcons,
		FileIcon: func(entry filetree.Entry[git.StageFileStatus]) string {
			return statusFileIcon(entry.Value, isWorktreeSymlink(m.worktreeRoot, entry.Value.Path), icons)
		},
		FileLabel: func(entry filetree.Entry[git.StageFileStatus]) string {
			if entry.Value.IsRenamed() && entry.Value.RenameFrom != "" {
				return entry.Value.RenameFrom + " -> " + entry.Value.Path
			}
			return entry.DisplayName
		},
		MetaText: func(entry filetree.Entry[git.StageFileStatus]) string {
			return statusEntryMeta(statusEntryFromRow(entry), m.settings.UseNerdFontIcons, icons)
		},
		RowColor: func(entry filetree.Entry[git.StageFileStatus]) string {
			return statusEntryColor(statusEntryFromRow(entry))
		},
		Faint: func(entry filetree.Entry[git.StageFileStatus]) bool {
			return entry.Kind == filetree.EntryFile && isDeletedFileStatus(entry.Value)
		},
	})
}

func (m Model) requiredFiletreePaneWidth(height int) int {
	required := ansi.StringWidth(" " + m.filetreePaneTitle() + " ")
	icons := filetreePaneIconsFor(m.settings.UseNerdFontIcons)
	if w := m.fileTreeModel.RequiredWidth(height, filetree.RenderOpts[git.StageFileStatus]{
		AccentColor:      ui.ColorBlue,
		Active:           m.focus == focusFiletree,
		EmptyLine:        ui.StyleMuted.Render("clean working tree"),
		UseNerdFontIcons: m.settings.UseNerdFontIcons,
		FileIcon: func(entry filetree.Entry[git.StageFileStatus]) string {
			return statusFileIcon(entry.Value, isWorktreeSymlink(m.worktreeRoot, entry.Value.Path), icons)
		},
		FileLabel: func(entry filetree.Entry[git.StageFileStatus]) string {
			if entry.Value.IsRenamed() && entry.Value.RenameFrom != "" {
				return entry.Value.RenameFrom + " -> " + entry.Value.Path
			}
			return entry.DisplayName
		},
		MetaText: func(entry filetree.Entry[git.StageFileStatus]) string {
			return statusEntryMeta(statusEntryFromRow(entry), m.settings.UseNerdFontIcons, icons)
		},
		RowColor: func(entry filetree.Entry[git.StageFileStatus]) string {
			return statusEntryColor(statusEntryFromRow(entry))
		},
		Faint: func(entry filetree.Entry[git.StageFileStatus]) bool {
			return entry.Kind == filetree.EntryFile && isDeletedFileStatus(entry.Value)
		},
	}); w > required {
		required = w
	}
	return required + 2
}

func (m Model) renderFiletreePane(width, height int) string {
	lines := m.visibleStatusLines(width, height)
	return m.renderFiletreePanelWithBorderTitle(width, height, m.filetreePaneTitle(), m.searchCounterForFiletreePane(), lines, m.focus == focusFiletree)
}

func (m Model) branchSummaryTitleSuffix() string {
	if strings.TrimSpace(m.statusData.branchName) == "" {
		return ""
	}
	branchLabel := "branch"
	if m.settings.UseNerdFontIcons {
		branchLabel = ""
	}
	out := branchLabel + " " + m.statusData.branchName + " " + m.branchSyncToken()
	base := strings.TrimSpace(m.statusData.branchBaseRef)
	if shouldShowBranchBaseRef(base) {
		out += " · vs " + base
	}
	return out
}

func (m Model) branchSyncToken() string {
	switch m.statusData.branchSync.Name {
	case git.StatusSame:
		return "✓"
	case git.StatusAhead:
		return fmt.Sprintf("↑%d", m.statusData.branchSync.Ahead)
	case git.StatusBehind:
		return fmt.Sprintf("↓%d", m.statusData.branchSync.Behind)
	case git.StatusDiverged:
		return fmt.Sprintf("↑%d ↓%d", m.statusData.branchSync.Ahead, m.statusData.branchSync.Behind)
	}
	return "?"
}

func shouldShowBranchBaseRef(base string) bool {
	base = strings.TrimSpace(base)
	if base == "" {
		return false
	}
	return base != "origin/main" && base != "origin/master"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
