package commit

import (
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui/tree"
)

func (m Model) InputFocused() bool {
	return m.fileTreeModel.Search().InputFocused() || m.diffModel.Search().InputFocused() || m.help.InputFocused()
}

func (m Model) fileEntrySearchText(entry tree.Entry[stage.CommitFile]) string {
	if entry.HasChildren {
		return entry.DisplayName + "/"
	}
	if entry.Value.RenameFrom != "" {
		return entry.Value.RenameFrom + " -> " + entry.Value.Path
	}
	return entry.Value.Path
}
