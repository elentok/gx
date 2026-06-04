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
