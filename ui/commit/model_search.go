package commit

func (m Model) InputFocused() bool {
	return m.sidebar.fileTreeModel.Search().InputFocused() || m.diff.diffModel.Search().InputFocused() || m.help.InputFocused()
}
