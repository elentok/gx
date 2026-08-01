package tickets

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// flatTranscriptSeparatorLines is the one line syncPreviewViewport reserves
// for previewLines' rule between the ticket body and the transcript tail
// (see flat_view.go's previewLines).
const flatTranscriptSeparatorLines = 1

// flatTranscriptMaxHeight caps how much of the preview panel's height a live
// ticket's transcript tail can claim, leaving most of the budget to the
// ticket body even on a tall terminal.
const flatTranscriptMaxHeight = 8

// splitPreviewHeight divides the preview panel's usable height between
// previewVP (the ticket body) and transcriptVP (the live transcript tail,
// ticket 04b) — the tail only appears at all when isLive, per ticket 03's
// unchanged done/open shape.
func splitPreviewHeight(height int, isLive bool) (bodyHeight, transcriptHeight int) {
	if !isLive {
		return height, 0
	}
	transcriptHeight = max(min(flatTranscriptMaxHeight, height/3), 1)
	bodyHeight = max(height-transcriptHeight-flatTranscriptSeparatorLines, 1)
	return
}

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

	_, isLive := m.liveStateForSelected()
	bodyHeight, transcriptHeight := splitPreviewHeight(height, isLive)

	m.previewVP.SetWidth(contentW)
	m.previewVP.SetHeight(bodyHeight)
	m.transcriptVP.SetWidth(contentW)
	m.transcriptVP.SetHeight(transcriptHeight)

	key := m.previewSelectionKey()
	selectionChanged := key != m.previewSelKey
	m.previewSelKey = key

	content := m.previewContent(contentW)
	if m.previewSearch.HasQuery() {
		content = highlightPreviewContent(content, m.previewSearch)
	}
	m.previewVP.SetContent(content)

	if isLive {
		if t, ok := m.selectedTicket(); ok {
			m.transcriptVP.SetContent(strings.Join(m.transcript[t.Identifier], "\n"))
		}
		m.transcriptVP.GotoBottom()
	} else {
		m.transcriptVP.SetContent("")
	}

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
