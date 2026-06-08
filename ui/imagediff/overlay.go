package imagediff

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/ui/kittygraphics"

	tea "charm.land/bubbletea/v2"
)

// imageExtensions is the file-extension allowlist for image-diff candidates.
var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
}

// HasImageExtension reports whether path's extension is in the image-diff
// allowlist. It is the first (cheapest) gate of image-diff eligibility, shared
// by every host.
func HasImageExtension(path string) bool {
	return imageExtensions[strings.ToLower(filepath.Ext(path))]
}

// SettleDebounce is how long the overlay waits after the last disrupting event
// before computing and placing a new image-diff overlay (ADR 0010's lifecycle
// rule — short enough to feel immediate, long enough to absorb a stream of j/k
// movement without thrashing).
const SettleDebounce = 80 * time.Millisecond

// SettleMsg fires when the debounce timer started by the most recent disrupting
// event elapses. Seq is compared against Overlay.settleSeq so stale timers
// (superseded by a later disrupting event) are ignored. It is exported so host
// containers that don't broadcast unknown messages (log, stash) can forward it
// explicitly to the panel that owns the overlay.
type SettleMsg struct{ Seq int }

// Overlay is the reusable controller for the inline image-diff kitty overlay
// (ADR 0010). It owns the eager-clear / debounced-replace lifecycle, the settle
// timer, the cached terminal capability, and the place/clear commands. It is
// embedded by any diff panel that opts in (status, commit) and parameterized by
// host callbacks supplied at construction (write bytes, detect capability) plus
// a SettleHost passed at settle time (selection, geometry, blobs, modal state).
//
// The host that paints the overlay must report its diff body geometry in
// absolute screen cells; a panel composed into a split view learns its origin
// from the container (see the "Screen origin" glossary entry).
type Overlay struct {
	// dirty is set by any disrupting event handler and consumed centrally by the
	// host's Update wrapper (which then calls Disrupt) — this keeps the "clear
	// eagerly, replace on settle" rule in one place rather than scattering
	// tea.Cmd plumbing across every event handler.
	dirty bool

	activeIDs []uint32
	nextID    uint32
	settleSeq int

	// fallbackPath is the path of the file for which the most recent settle
	// determined Plan returns a fallback (decode failure, oversized, etc). The
	// host's View shows the binary summary line for this path instead of
	// reserving space, until the selection moves elsewhere.
	fallbackPath string

	capability         kittygraphics.Capability
	capabilityDetected bool

	writeBytes       func(data []byte)
	detectCapability func() kittygraphics.Capability
}

// NewOverlay builds an Overlay wired to host-specific I/O: writeBytes pushes
// raw kitty escape sequences to the terminal (outside bubbletea's render loop),
// and detectCapability probes the host terminal's graphics support once.
func NewOverlay(writeBytes func(data []byte), detectCapability func() kittygraphics.Capability) Overlay {
	return Overlay{
		writeBytes:       writeBytes,
		detectCapability: detectCapability,
	}
}

// MarkDirty flags a disrupting event. The host's Update wrapper consumes it via
// Disrupt.
func (o *Overlay) MarkDirty() { o.dirty = true }

// Dirty reports whether a disrupting event has been flagged since the last
// Disrupt.
func (o Overlay) Dirty() bool { return o.dirty }

// FallbackPath returns the path the most recent settle marked as fallback (the
// host renders the binary summary line for it instead of reserving overlay
// space).
func (o Overlay) FallbackPath() string { return o.fallbackPath }

// HasActivePlacements reports whether any kitty placements are currently on
// screen.
func (o Overlay) HasActivePlacements() bool { return len(o.activeIDs) > 0 }

// SettleSeq returns the current settle-debounce sequence number, so callers can
// construct a matching SettleMsg (used by tests that bypass the real timer).
func (o Overlay) SettleSeq() int { return o.settleSeq }

// Capability detects (and caches) the host terminal's kitty-graphics
// capability, mirroring ui.DetectTerminal's caching of $KITTY_*/$TMUX checks —
// detection runs at most once per Overlay value lineage.
func (o *Overlay) Capability() kittygraphics.Capability {
	if !o.capabilityDetected {
		o.capability = o.detectCapability()
		o.capabilityDetected = true
	}
	return o.capability
}

// Disrupt implements ADR 0010's eager-clear / debounced-replace lifecycle rule.
// It is invoked centrally from the host's Update whenever dirty was set by a
// disrupting event: any active placements are cleared immediately and
// unconditionally, and (when enabled) a new settle debounce is (re)started so a
// fresh placement can be computed once the model stops moving. enabled mirrors
// the host's image-diffs config toggle.
func (o Overlay) Disrupt(enabled bool) (Overlay, tea.Cmd) {
	o.dirty = false

	var clearCmd tea.Cmd
	if len(o.activeIDs) > 0 {
		clearCmd = o.cmdClear(o.activeIDs)
		o.activeIDs = nil
	}

	if !enabled {
		return o, clearCmd
	}

	o.settleSeq++
	seq := o.settleSeq
	settleCmd := tea.Tick(SettleDebounce, func(time.Time) tea.Msg {
		return SettleMsg{Seq: seq}
	})
	return o, tea.Batch(clearCmd, settleCmd)
}

// SettleHost supplies the model-state-dependent inputs the overlay needs at
// place time. The host implements it (usually the diff panel's Model value);
// the overlay calls the methods lazily and in order so expensive work (blob
// fetch) only runs once the cheap gates pass.
type SettleHost interface {
	// SelectedImageFile returns the selected file's path and whether it is an
	// image-diff candidate (extension allowlist + config enabled + terminal
	// supported). ("", false) skips placement entirely.
	SelectedImageFile() (path string, ok bool)
	// ModalOpen reports whether a modal currently occludes the diff panel — a
	// kitty placement would paint over it at the graphics layer, so the overlay
	// waits for the modal to close (which is itself a disrupting event).
	ModalOpen() bool
	// PanelGeometry returns the absolute screen rect of the diff body: the
	// top-left cell plus the available column/row span.
	PanelGeometry() (originCol, originRow, availCols, availRows int, ok bool)
	// FetchBlobs returns the old/new image bytes for the selected file. oldOK /
	// newOK report side presence (false for the absent side of an added/deleted
	// file — an expected state, not an error).
	FetchBlobs() (old, newBytes []byte, oldOK, newOK bool)
}

// HandleSettle runs once the debounce timer from the most recent disrupting
// event elapses. If the model has settled (msg.Seq still current), the selection
// is still an image-diff candidate, no modal is open, and geometry is stable, it
// fetches the blobs, computes a layout plan, and either emits a placement
// command or records the file as a fallback (rendered as the binary summary line
// by the host's View).
func (o Overlay) HandleSettle(msg SettleMsg, host SettleHost) (Overlay, tea.Cmd) {
	if msg.Seq != o.settleSeq {
		return o, nil
	}

	path, ok := host.SelectedImageFile()
	if !ok {
		return o, nil
	}

	// A modal overlays the diff panel as text composed into View()'s output
	// (ui.OverlayCenter), but a kitty placement paints over that at the
	// terminal's graphics layer regardless — so placing here would occlude the
	// modal. Bail without touching fallbackPath: modal-open says nothing about
	// whether this file is fallback-worthy, and the modal closing re-marks the
	// overlay dirty, triggering a fresh settle that re-evaluates from scratch.
	if host.ModalOpen() {
		return o, nil
	}

	originCol, originRow, availCols, availRows, ok := host.PanelGeometry()
	if !ok {
		return o, nil
	}

	old, newBytes, oldOK, newOK := host.FetchBlobs()
	if !oldOK && !newOK {
		o.fallbackPath = path
		return o, nil
	}

	capability := o.Capability()
	plan := Plan(old, newBytes, availCols, availRows, capability.PixelsPerCol, capability.PixelsPerRow)
	if plan.Fallback {
		o.fallbackPath = path
		return o, nil
	}

	o.fallbackPath = ""
	return o.place(plan, old, newBytes, originCol, originRow)
}

// OnDeactivate clears any active placements when the host page is switched away
// from — a disrupting event per ADR 0010, since the overlay would otherwise be
// left floating over whatever the next page renders. Clearing is eager and
// unconditional.
func (o Overlay) OnDeactivate() tea.Cmd {
	if len(o.activeIDs) == 0 {
		return nil
	}
	return o.cmdClear(o.activeIDs)
}

// cmdClear returns a tea.Cmd that writes the kitty graphics-protocol delete
// sequences for ids directly to the terminal, as a side effect outside
// bubbletea's render loop (ADR 0010).
func (o Overlay) cmdClear(ids []uint32) tea.Cmd {
	capability := o.capability
	write := o.writeBytes
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

// place allocates placement IDs for the plan's sides, records them as active (so
// the next disrupting event clears them), and returns a tea.Cmd that moves the
// cursor to each placement's absolute screen position and writes the kitty
// graphics-protocol transmit-and-display sequence.
func (o Overlay) place(plan RenderPlan, old, newBytes []byte, originCol, originRow int) (Overlay, tea.Cmd) {
	capability := o.capability
	write := o.writeBytes

	type pendingPlacement struct {
		id                 uint32
		data               []byte
		col, row           int
		spanCols, spanRows int
	}
	var pending []pendingPlacement
	add := func(p *Placement, data []byte) {
		if p == nil || len(data) == 0 {
			return
		}
		o.nextID++
		pending = append(pending, pendingPlacement{
			id:       o.nextID,
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
		return o, nil
	}

	ids := make([]uint32, 0, len(pending))
	for _, p := range pending {
		ids = append(ids, p.id)
	}
	o.activeIDs = ids

	return o, func() tea.Msg {
		var out []byte
		for _, p := range pending {
			out = append(out, ansiCursorPosition(p.row, p.col)...)
			out = append(out, kittygraphics.EncodePlacement(capability, p.id, p.data, p.spanCols, p.spanRows)...)
		}
		write(out)
		return nil
	}
}

// ansiCursorPosition returns the CSI sequence that moves the cursor to the given
// 0-based screen row/column (the kitty graphics protocol places images at the
// current cursor position).
func ansiCursorPosition(row, col int) []byte {
	return fmt.Appendf(nil, "\033[%d;%dH", row+1, col+1)
}
