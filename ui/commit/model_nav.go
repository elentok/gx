package commit

// CurrentRef returns the commit ref currently displayed by this model.
func (m Model) CurrentRef() string {
	return m.ref
}

// HasInternalFocus reports whether the commit model has a focused sub-panel
// (diff or header) that esc should step back through before the caller acts.
func (m Model) HasInternalFocus() bool {
	return m.focusDiff || m.focusHeader || m.fileTreeModel.Search().IsActive()
}

// IsFileTreeFocused reports whether the commit file tree is the active
// sub-panel. Search input focus is excluded so typed characters are preserved.
func (m Model) IsFileTreeFocused() bool {
	return !m.focusDiff && !m.focusHeader && !m.fileTreeModel.Search().InputFocused()
}

// IsHeaderFocused reports whether the commit info/header panel owns internal
// focus.
func (m Model) IsHeaderFocused() bool {
	return m.focusHeader
}

// DiffScrollOffset returns the diff viewport's current vertical scroll
// offset, for callers (e.g. hover-routing tests) that need to observe
// whether a wheel event reached the diff pane.
func (m Model) DiffScrollOffset() int {
	return m.diffModel.Viewport().YOffset()
}

// FileTreeScrollOffset returns the file tree's current vertical scroll
// offset, for callers (e.g. hover-routing tests) that need to observe
// whether a wheel event reached the file tree.
func (m Model) FileTreeScrollOffset() int {
	return m.fileTreeModel.ScrollOffset()
}
