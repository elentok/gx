package commit

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/filetree"
)

func (m Model) InputFocused() bool {
	return m.fileTreeModel.Search().InputFocused() || m.diffModel.Search().InputFocused()
}

func (m Model) fileEntrySearchText(entry filetree.Entry[git.CommitFile]) string {
	if entry.Kind == filetree.EntryDir {
		return entry.DisplayName + "/"
	}
	if entry.Value.RenameFrom != "" {
		return entry.Value.RenameFrom + " -> " + entry.Value.Path
	}
	return entry.Value.Path
}

