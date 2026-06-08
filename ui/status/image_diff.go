package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/kittygraphics"
	"github.com/elentok/gx/ui/status/diffarea"
	"github.com/elentok/gx/ui/status/imagediff"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/sys/unix"
)

// imageDiffSettleDebounce is how long the model waits after the last
// disrupting event before computing and placing a new image-diff overlay
// (ADR 0010's lifecycle rule — short enough to feel immediate, long enough to
// absorb a stream of j/k movement without thrashing).
const imageDiffSettleDebounce = 80 * time.Millisecond

// imageDiffExtensions is the file-extension allowlist for image-diff
// candidates (PRD "Modules" §4).
var imageDiffExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
}

// imageDiffSettleMsg fires when the debounce timer started by the most recent
// disrupting event elapses. seq is compared against imageDiffState.settleSeq
// so stale timers (superseded by a later disrupting event) are ignored.
type imageDiffSettleMsg struct{ seq int }

// imageDiffState tracks the inline image-diff overlay's lifecycle: which
// placements (if any) are currently on screen, the in-flight settle debounce,
// and the cached terminal capability (queried once, per ADR 0010).
type imageDiffState struct {
	// dirty is set by any disrupting event handler and consumed centrally in
	// Update, which then calls disruptImageDiff — this keeps the "clear
	// eagerly, replace on settle" rule in one place rather than scattering
	// tea.Cmd plumbing across every event handler.
	dirty bool

	activeIDs []uint32
	nextID    uint32
	settleSeq int

	// fallbackPath is the path of the file for which the most recent settle
	// determined imagediff.Plan returns a fallback (decode failure, oversized,
	// etc). View() shows binarySummaryLine for this path instead of reserving
	// space, until the selection moves elsewhere.
	fallbackPath string

	capability         kittygraphics.Capability
	capabilityDetected bool
}

func hasImageDiffExtension(path string) bool {
	return imageDiffExtensions[strings.ToLower(filepath.Ext(path))]
}

// imageDiffCapability detects (and caches) the host terminal's kitty-graphics
// capability, mirroring ui.DetectTerminal's caching of $KITTY_*/$TMUX checks —
// detection runs at most once per Model lifetime.
func (m *Model) imageDiffCapability() kittygraphics.Capability {
	if !m.imageDiff.capabilityDetected {
		m.imageDiff.capability = m.detectImageDiffCapability()
		m.imageDiff.capabilityDetected = true
	}
	return m.imageDiff.capability
}

// queryTerminalWinSize reads the terminal's cell grid and pixel dimensions via
// TIOCGWINSZ on stdout, used for aspect-correct image scaling.
func queryTerminalWinSize() (kittygraphics.WinSize, bool) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return kittygraphics.WinSize{}, false
	}
	return kittygraphics.WinSize{
		Cols:        int(ws.Col),
		Rows:        int(ws.Row),
		PixelWidth:  int(ws.Xpixel),
		PixelHeight: int(ws.Ypixel),
	}, true
}

// probeKittyGraphics never actually probes the terminal: bubbletea owns
// stdin's read loop, and writing a query then blocking for a response would
// race with (and corrupt) its input handling. Without a probe, a tmux-hosted
// kitty terminal is detected as unsupported — the documented graceful
// fallback for that setup (PRD user story 7).
func probeKittyGraphics(string) (response string, ok bool) {
	return "", false
}

// isImageDiffEligible reports whether file is a candidate for inline
// image-diff rendering: its extension is in the allowlist, the image-diffs
// config option is enabled, and the host terminal supports the kitty graphics
// protocol. All three must hold — otherwise behavior is exactly today's
// (binarySummaryLine).
func (m *Model) isImageDiffEligible(file statusDiffFileSelection) bool {
	if !m.settings.ImageDiffs {
		return false
	}
	if !hasImageDiffExtension(file.Path) {
		return false
	}
	return m.imageDiffCapability().Supported
}

// appendBinaryDiffLines appends the lines to render in place of a binary diff
// with no view lines. When the selected file is image-diff-eligible (and its
// most recent settle didn't determine a fallback), it reserves bodyH blank
// lines so bubbletea's layout math accounts for the overlay area — the actual
// graphics are placed as a side effect of a tea.Cmd, never embedded here
// (ADR 0010). Otherwise it falls back to the single binarySummaryLine, exactly
// as before this feature existed.
func (m *Model) appendBinaryDiffLines(lines []string, bodyH, innerW int) []string {
	if file, ok := m.selectedStatusFile(); ok && m.isImageDiffEligible(file) && m.imageDiff.fallbackPath != file.Path {
		blank := strings.Repeat(" ", innerW)
		for range bodyH {
			lines = append(lines, blank)
		}
		return lines
	}
	return append(lines, lipgloss.NewStyle().Foreground(ui.ColorSubtle).Render(m.binarySummaryLine()))
}

// disruptImageDiff implements ADR 0010's eager-clear / debounced-replace
// lifecycle rule. It is invoked centrally from Update whenever
// imageDiffState.dirty was set by a disrupting event (selection change,
// scroll, resize, focus change, fullscreen toggle): any active placements are
// cleared immediately and unconditionally, and a new settle debounce is
// (re)started so a fresh placement can be computed once the model stops
// moving.
func (m *Model) disruptImageDiff() tea.Cmd {
	var clearCmd tea.Cmd
	if len(m.imageDiff.activeIDs) > 0 {
		clearCmd = m.cmdClearImagePlacements(m.imageDiff.activeIDs)
		m.imageDiff.activeIDs = nil
	}

	if !m.settings.ImageDiffs {
		return clearCmd
	}

	m.imageDiff.settleSeq++
	seq := m.imageDiff.settleSeq
	settleCmd := tea.Tick(imageDiffSettleDebounce, func(time.Time) tea.Msg {
		return imageDiffSettleMsg{seq: seq}
	})
	return tea.Batch(clearCmd, settleCmd)
}

// handleImageDiffSettle runs once the debounce timer from the most recent
// disrupting event elapses. If the model has settled (msg.seq still current)
// and the selected file is still image-diff-eligible, it fetches the old/new
// blobs, computes a layout plan, and either emits a placement command or
// records the file as a fallback (shown as binarySummaryLine by View).
func (m Model) handleImageDiffSettle(msg imageDiffSettleMsg) (Model, tea.Cmd) {
	if msg.seq != m.imageDiff.settleSeq {
		return m, nil
	}

	file, ok := m.selectedStatusFile()
	if !ok || !m.isImageDiffEligible(file) {
		return m, nil
	}

	originCol, originRow, availCols, availRows, ok := m.imageDiffPanelGeometry()
	if !ok {
		return m, nil
	}

	cached := m.diffarea.ActiveSection == diffarea.SectionStaged
	old, newBytes, oldOK, newOK := m.fetchImageDiffBlobs(file.stageFile, cached)
	if !oldOK && !newOK {
		m.imageDiff.fallbackPath = file.Path
		return m, nil
	}

	capability := m.imageDiffCapability()
	plan := imagediff.Plan(old, newBytes, availCols, availRows, capability.PixelsPerCol, capability.PixelsPerRow)
	if plan.Fallback {
		m.imageDiff.fallbackPath = file.Path
		return m, nil
	}

	m.imageDiff.fallbackPath = ""
	return m, m.cmdPlaceImageDiff(plan, old, newBytes, originCol, originRow)
}

// imageDiffPanelGeometry computes the diff panel's currently active section's
// body area in absolute screen cells (origin column/row plus available
// columns/rows) — this is only stable once layout has settled, which is why
// it's computed at place-time rather than cached.
func (m Model) imageDiffPanelGeometry() (originCol, originRow, availCols, availRows int, ok bool) {
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

// cmdClearImagePlacements returns a tea.Cmd that writes the kitty
// graphics-protocol delete sequences for ids directly to the terminal, as a
// side effect outside bubbletea's render loop (ADR 0010).
func (m *Model) cmdClearImagePlacements(ids []uint32) tea.Cmd {
	capability := m.imageDiff.capability
	write := m.writeImageDiffBytes
	idsCopy := append([]uint32(nil), ids...)
	return func() tea.Msg {
		var out []byte
		for _, id := range idsCopy {
			out = append(out, kittygraphics.EncodeClear(capability, id)...)
		}
		write(out)
		return nil
	}
}

// cmdPlaceImageDiff allocates placement IDs for the plan's sides, records them
// as active (so the next disrupting event clears them), and returns a tea.Cmd
// that moves the cursor to each placement's absolute screen position and
// writes the kitty graphics-protocol transmit-and-display sequence.
func (m *Model) cmdPlaceImageDiff(plan imagediff.RenderPlan, old, newBytes []byte, originCol, originRow int) tea.Cmd {
	capability := m.imageDiff.capability
	write := m.writeImageDiffBytes

	type pendingPlacement struct {
		id                 uint32
		data               []byte
		col, row           int
		spanCols, spanRows int
	}
	var pending []pendingPlacement
	add := func(p *imagediff.Placement, data []byte) {
		if p == nil || len(data) == 0 {
			return
		}
		m.imageDiff.nextID++
		pending = append(pending, pendingPlacement{
			id:       m.imageDiff.nextID,
			data:     data,
			col:      originCol + p.Col,
			row:      originRow + p.Row,
			spanCols: p.SpanCols,
			spanRows: p.SpanRows,
		})
	}
	add(plan.Old, old)
	add(plan.New, newBytes)

	if len(pending) == 0 {
		return nil
	}

	ids := make([]uint32, 0, len(pending))
	for _, p := range pending {
		ids = append(ids, p.id)
	}
	m.imageDiff.activeIDs = ids

	return func() tea.Msg {
		var out []byte
		for _, p := range pending {
			out = append(out, ansiCursorPosition(p.row, p.col)...)
			out = append(out, kittygraphics.EncodePlacement(capability, p.id, p.data, p.spanCols, p.spanRows)...)
		}
		write(out)
		return nil
	}
}

// ansiCursorPosition returns the CSI sequence that moves the cursor to the
// given 0-based screen row/column (the kitty graphics protocol places images
// at the current cursor position).
func ansiCursorPosition(row, col int) []byte {
	return fmt.Appendf(nil, "\033[%d;%dH", row+1, col+1)
}
