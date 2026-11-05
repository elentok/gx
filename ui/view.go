package ui

import (
	tea "charm.land/bubbletea/v2"
)

// NewMainView creates a view with the standard flags for top-level page views.
func NewMainView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.ReportFocus = true
	return v
}

// NewHoverView is NewMainView in all-motion mode, so the terminal reports
// motion with no button held.
func NewHoverView(content string) tea.View {
	v := NewMainView(content)
	v.MouseMode = tea.MouseModeAllMotion
	return v
}
