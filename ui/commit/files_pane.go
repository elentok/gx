package commit

import (
	"image/color"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/tree"
)

// filesPane owns the changed-file list and its tree. Model owns the inputs and
// passes them in as a filesInput on every render call. The methods that move
// the selection report whether it moved; Model refreshes the diff.
type filesPane struct {
	files         []stage.CommitFile
	fileTreeModel tree.Model[stage.CommitFile]
}

// filesInput is the Model state the files pane reads. active is true when the
// container owns keyboard focus and neither the diff nor the header does.
type filesInput struct {
	width     int
	active    bool
	focusDiff bool
	nerdFont  bool
}

func (m Model) filesInput() filesInput {
	return filesInput{
		width:     m.width,
		active:    !m.inactive && !m.focusDiff && !m.focusHeader,
		focusDiff: m.focusDiff,
		nerdFont:  m.settings.UseNerdFontIcons,
	}
}

func (p filesPane) selectedCommitEntry() (tree.Entry[stage.CommitFile], bool) {
	entries := p.fileTreeModel.Entries()
	selected := p.fileTreeModel.SelectedIndex()
	if selected < 0 || selected >= len(entries) {
		return tree.Entry[stage.CommitFile]{}, false
	}
	return entries[selected], true
}

func (p filesPane) selectedCommitFile() (stage.CommitFile, bool) {
	entry, ok := p.selectedCommitEntry()
	if !ok || entry.HasChildren {
		return stage.CommitFile{}, false
	}
	return entry.Value, true
}

func (p *filesPane) selectFirstCommitFile() {
	entries := p.fileTreeModel.Entries()
	for i, entry := range entries {
		if !entry.HasChildren {
			p.fileTreeModel.SetSelectedIndex(i)
			return
		}
	}
}

func (p *filesPane) moveToAdjacentFile(delta int) bool {
	return p.fileTreeModel.MoveToAdjacentFile(delta)
}

func (p *filesPane) jumpSidebarTop() bool {
	if len(p.fileTreeModel.Entries()) == 0 || p.fileTreeModel.SelectedIndex() == 0 {
		return false
	}
	p.fileTreeModel.SetSelectedIndex(0)
	return true
}

func (p *filesPane) jumpSidebarBottom() bool {
	entries := p.fileTreeModel.Entries()
	if len(entries) == 0 || p.fileTreeModel.SelectedIndex() == len(entries)-1 {
		return false
	}
	p.fileTreeModel.SetSelectedIndex(len(entries) - 1)
	return true
}

func (p *filesPane) scrollSidebarPage(direction int) bool {
	prev := p.fileTreeModel.SelectedIndex()
	p.fileTreeModel.ScrollPage(direction * list.DefaultScroll)
	return p.fileTreeModel.SelectedIndex() != prev
}

func (p *filesPane) rebuildCommitFiletree() {
	entries := tree.BuildEntriesFromPaths(
		p.files,
		func(file stage.CommitFile) string { return file.Path },
		p.fileTreeModel.CollapsedIDs(),
	)
	p.fileTreeModel.SetEntries(entries)
}

func (filesPane) fileEntrySearchText(entry tree.Entry[stage.CommitFile]) string {
	if entry.HasChildren {
		return entry.DisplayName + "/"
	}
	if entry.Value.RenameFrom != "" {
		return entry.Value.RenameFrom + " -> " + entry.Value.Path
	}
	return entry.Value.Path
}

func (p filesPane) renderFilesPane(in filesInput, width, height int) string {
	lines := p.visibleFileLines(in, width, height)
	if len(lines) == 0 {
		lines = append(lines, ui.StyleMuted.Render("no changed files"))
	}
	title, rightTitle := "Files", p.filesPaneRightTitle(in)
	titleColor := p.filesPaneTitleColor(in)
	accent := color.Color(nil)
	if in.active {
		accent = titleColor
	}
	return ui.RenderPanel(ui.PanelOptionsFor(width, height, title, rightTitle, lines, in.active, titleColor, accent, false))
}

func (p filesPane) visibleFileLines(in filesInput, width, height int) []string {
	opts := p.filetreeRenderOpts(in)
	opts.Width = width - 2
	return p.fileTreeModel.RenderLines(height, opts)
}

func (p filesPane) requiredFilesPaneWidth(in filesInput, height int) int {
	required := ansi.StringWidth(" Files ")
	if w := p.fileTreeModel.RequiredWidth(height, p.filetreeRenderOpts(in)); w > required {
		required = w
	}
	// 2 for the frame + 1 for padding
	return required + 2 + 1
}

func (filesPane) filetreeRenderOpts(in filesInput) tree.RenderOpts[stage.CommitFile] {
	icons := ui.Icons(in.nerdFont)
	return tree.RenderOpts[stage.CommitFile]{
		AccentColor: ui.ColorOrange,
		Active:      in.active,
		EmptyLine:   ui.StyleMuted.Render("no changed files"),
		Icon: func(entry tree.Entry[stage.CommitFile]) string {
			if entry.HasChildren {
				if entry.Expanded {
					return icons.FolderOpen
				}
				return icons.FolderClosed
			}
			return commitFileIcon(entry.Value, in.nerdFont)
		},
		Label: func(entry tree.Entry[stage.CommitFile]) string {
			if entry.HasChildren {
				return entry.DisplayName + "/"
			}
			if entry.Value.RenameFrom != "" {
				return entry.Value.RenameFrom + " -> " + entry.Value.Path
			}
			return entry.DisplayName
		},
		MetaText: func(entry tree.Entry[stage.CommitFile]) string {
			return commitEntryMeta(entry, in.nerdFont)
		},
		RowColor: func(entry tree.Entry[stage.CommitFile]) string {
			return commitEntryColor(entry)
		},
	}
}

func (p filesPane) filesPaneWidth(in filesInput, height int) int {
	width := max(FILE_TREE_MIN_WIDTH, min(FILE_TREE_MAX_WIDTH, p.requiredFilesPaneWidth(in, height)))
	if in.width-width < DIFF_MIN_WIDTH {
		width = in.width - DIFF_MIN_WIDTH
	}
	if width < FILE_TREE_MIN_WIDTH {
		width = FILE_TREE_MIN_WIDTH
	}
	return width
}

func (filesPane) filesPaneTitleColor(in filesInput) color.Color {
	if !in.active {
		return ui.ColorBlue
	}
	return ui.ColorOrange
}

func (filesPane) filesPaneBorderColor(in filesInput) color.Color {
	if !in.active {
		return ui.ColorBorder
	}
	return ui.ColorOrange
}

func (p filesPane) filesPaneRightTitle(in filesInput) string {
	if in.focusDiff {
		return ""
	}
	if len(p.fileTreeModel.Entries()) == 0 {
		return ""
	}
	return "tree"
}
