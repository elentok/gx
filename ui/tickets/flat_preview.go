package tickets

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
)

// previewSelectionKey identifies which ticket the preview is currently
// showing, mirroring Model.previewSelectionKey (model_preview_focus.go).
func (m FlatModel) previewSelectionKey() string {
	if !m.found {
		return ""
	}
	return "ticket:" + strconv.Itoa(m.selected)
}

// syncPreviewViewport keeps m.previewVP's size/content aligned with the
// current layout/selection, mirroring Model.syncPreviewViewport.
func (m *FlatModel) syncPreviewViewport() {
	if !m.ready {
		return
	}
	_, previewW := m.flatSplitWidth()
	h := m.flatContentHeight()
	width, height := previewInnerSize(previewW, h)
	contentW := max(width-previewScrollbarGutter, 1)

	m.previewVP.SetWidth(contentW)
	m.previewVP.SetHeight(height)

	key := m.previewSelectionKey()
	selectionChanged := key != m.previewSelKey
	m.previewSelKey = key

	content := m.previewContent(contentW)
	if m.previewSearch.HasQuery() {
		content = highlightPreviewContent(content, m.previewSearch)
	}
	m.previewVP.SetContent(content)

	if selectionChanged {
		m.previewVP.GotoTop()
		m.previewSearch.SetMatches(nil)
	}
}

func (m FlatModel) previewMatchStatus() string {
	return previewSearchMatchStatus(m.previewSearch)
}

// handleFlatPreviewKey processes key input while the preview panel has
// focus, mirroring Model.handlePreviewKey (model_preview_focus.go): its own
// search overlay, "h"/"left"/"esc" handing focus back to the list, and
// everything else delegated to the viewport's own scrolling.
func (m FlatModel) handleFlatPreviewKey(msg tea.KeyPressMsg) (FlatModel, tea.Cmd) {
	if handled, cmd := updatePreviewSearchKey(msg, &m.previewVP, &m.previewSearch); handled {
		return m, cmd
	}

	if msg.String() == "q" {
		return m, tea.Quit
	}

	switch msg.String() {
	case "h", "left", "esc":
		m.focus = flatFocusList
		return m, nil
	}

	var cmd tea.Cmd
	m.previewVP, cmd = m.previewVP.Update(msg)
	return m, cmd
}
