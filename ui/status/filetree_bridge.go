package status

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/filetree"
)

func (m *Model) syncFileTreeModel() {
	m.fileTreeModel.SetEntries(statusEntriesToFileTreeEntries(m.statusEntries))
	m.fileTreeModel.SetSelectedIndex(m.selected)
	m.selected = m.fileTreeModel.SelectedIndex()
}

func (m *Model) setStatusSelection(index int) {
	m.fileTreeModel.SetSelectedIndex(index)
	m.selected = m.fileTreeModel.SelectedIndex()
}

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
