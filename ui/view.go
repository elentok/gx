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
