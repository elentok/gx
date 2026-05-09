package status

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/filetree"
)

// reconcileFileTreeFromStatusState rebuilds filetree view state from status state.
//
// The final setStatusSelection call is required after SetEntries because row
// count/shape may have changed. It reapplies m.selected, clamps it to a valid
// index in fileTreeModel, and writes the clamped value back to m.selected so
// parent and child selection cannot drift.
func (m *Model) reconcileFileTreeFromStatusState() {
	m.fileTreeModel.SetCollapsedDirs(m.collapsedDirs)
	m.fileTreeModel.SetEntries(statusEntriesToFileTreeEntries(m.statusEntries))
	m.setStatusSelection(m.selected)
}

func (m *Model) setStatusSelection(index int) {
	m.fileTreeModel.SetSelectedIndex(index)
	m.selected = m.fileTreeModel.SelectedIndex()
}

// statusEntriesToFileTreeEntries is temporary migration glue while status keeps
// mirrored sidebar rows. It should be removed once filetree.Model is the sole
// source of truth for rows/selection/collapse state.
func statusEntriesToFileTreeEntries(entries []statusEntry) []filetree.Entry[git.StageFileStatus] {
	out := make([]filetree.Entry[git.StageFileStatus], 0, len(entries))
	for _, entry := range entries {
		row := filetree.Entry[git.StageFileStatus]{
			Path:        entry.Path,
			ParentPath:  entry.ParentPath,
			Depth:       entry.Depth,
			DisplayName: entry.DisplayName,
			Expanded:    entry.Expanded,
		}
		if entry.Kind == statusEntryDir {
			row.Kind = filetree.EntryDir
		} else {
			row.Kind = filetree.EntryFile
			row.Value = entry.File
		}
		out = append(out, row)
	}
	return out
}
