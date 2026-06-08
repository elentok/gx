package status

import (
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/imagediff"
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// isImageDiffEligible reports whether file is a candidate for inline
// image-diff rendering: its extension is in the allowlist, the image-diffs
// config option is enabled, and the host terminal supports the kitty graphics
// protocol. All three must hold — otherwise behavior is exactly today's
// (binarySummaryLine).
func (m *Model) isImageDiffEligible(file statusDiffFileSelection) bool {
	if !m.settings.ImageDiffs {
		return false
	}
	if !imagediff.HasImageExtension(file.Path) {
		return false
	}
	return m.overlay.Capability().Supported
}

// appendBinaryDiffLines appends the lines to render in place of a binary diff
// with no view lines. When the selected file is image-diff-eligible (and its
// most recent settle didn't determine a fallback), it reserves bodyH blank
// lines so bubbletea's layout math accounts for the overlay area — the actual
// graphics are placed as a side effect of a tea.Cmd, never embedded here
// (ADR 0010). Otherwise it falls back to the single binarySummaryLine, exactly
// as before this feature existed.
func (m *Model) appendBinaryDiffLines(lines []string, bodyH, innerW int) []string {
	if file, ok := m.selectedStatusFile(); ok && m.isImageDiffEligible(file) && m.overlay.FallbackPath() != file.Path {
		blank := strings.Repeat(" ", innerW)
		for range bodyH {
			lines = append(lines, blank)
		}
		return lines
	}
	return append(lines, lipgloss.NewStyle().Foreground(ui.ColorSubtle).Render(m.binarySummaryLine()))
}

// handleImageDiffSettle delegates the settled-placement decision to the overlay
// controller, passing the status Model itself as the SettleHost.
func (m Model) handleImageDiffSettle(msg imagediff.SettleMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.overlay, cmd = m.overlay.HandleSettle(msg, m)
	return m, cmd
}

// SelectedImageFile implements imagediff.SettleHost: it reports the selected
// file's path when (and only when) that file is an image-diff candidate.
func (m Model) SelectedImageFile() (string, bool) {
	file, ok := m.selectedStatusFile()
	if !ok || !m.isImageDiffEligible(file) {
		return "", false
	}
	return file.Path, true
}

// PanelGeometry implements imagediff.SettleHost: it returns the diff panel's
// currently active section's body area in absolute screen cells. Status owns the
// whole screen, so these are already screen coordinates (origin (0,0)). This is
// only stable once layout has settled, which is why it's computed at place-time
// rather than cached.
func (m Model) PanelGeometry() (originCol, originRow, availCols, availRows int, ok bool) {
	mainH := m.height - 1
	if mainH < 1 {
		return 0, 0, 0, 0, false
	}
	diffX, diffY, diffW, diffH, drOK := m.diffRect(mainH)
	if !drOK || diffW <= 2 || diffH <= 2 {
		return 0, 0, 0, 0, false
	}

	expandedH, collapsedH := diffPaneHeights(diffH)
	paneY, paneH := diffY, expandedH
	if m.diffarea.ActiveSection == diffarea.SectionStaged {
		paneY = diffY + collapsedH
	}
	if paneH <= 2 {
		return 0, 0, 0, 0, false
	}

	// The pane is rendered with a 1-cell border on every side; the body starts
	// just inside it (see renderSectionPane / renderPanelWithBorderTitle).
	return diffX + 1, paneY + 1, diffW - 2, paneH - 2, true
}

// FetchBlobs implements imagediff.SettleHost: it fetches the selected file's
// old/new image bytes following the active section's working-tree-vs-index rules.
func (m Model) FetchBlobs() (old, newBytes []byte, oldOK, newOK bool) {
	file, ok := m.selectedStatusFile()
	if !ok {
		return nil, nil, false, false
	}
	cached := m.diffarea.ActiveSection == diffarea.SectionStaged
	return m.fetchImageDiffBlobs(file.stageFile, cached)
}
