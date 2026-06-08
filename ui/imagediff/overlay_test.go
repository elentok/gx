package imagediff

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/elentok/gx/ui/kittygraphics"

	tea "charm.land/bubbletea/v2"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func supportedCapability() kittygraphics.Capability {
	return kittygraphics.Capability{Supported: true, PixelsPerCol: 10, PixelsPerRow: 20}
}

type fakeHost struct {
	path                                   string
	pathOK                                 bool
	modalOpen                              bool
	geomOK                                 bool
	origCol, origRow, availCols, availRows int
	old, newBytes                          []byte
	oldOK, newOK                           bool
	fetchCalls                             int
}

func (h *fakeHost) SelectedImageFile() (string, bool) { return h.path, h.pathOK }
func (h *fakeHost) ModalOpen() bool                   { return h.modalOpen }
func (h *fakeHost) PanelGeometry() (int, int, int, int, bool) {
	return h.origCol, h.origRow, h.availCols, h.availRows, h.geomOK
}
func (h *fakeHost) FetchBlobs() ([]byte, []byte, bool, bool) {
	h.fetchCalls++
	return h.old, h.newBytes, h.oldOK, h.newOK
}

func newTestOverlay(written *[]byte) Overlay {
	return NewOverlay(
		func(data []byte) { *written = append(*written, data...) },
		supportedCapability,
	)
}

func eligibleHost() *fakeHost {
	old := []byte{}
	return &fakeHost{
		path:      "a.png",
		pathOK:    true,
		geomOK:    true,
		availCols: 40,
		availRows: 20,
		old:       old,
		oldOK:     false,
		newOK:     true,
	}
}

// runCmd executes a single (non-batch, non-tick) command and returns its msg.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestHandleSettlePlacesOnEligibleFile(t *testing.T) {
	t.Parallel()
	var written []byte
	o := newTestOverlay(&written)
	host := eligibleHost()
	host.old = encodePNG(t, 8, 8)
	host.oldOK = true
	host.newBytes = encodePNG(t, 16, 8)

	o, cmd := o.HandleSettle(SettleMsg{Seq: 0}, host)
	runCmd(t, cmd)

	if host.fetchCalls != 1 {
		t.Fatalf("expected exactly one blob fetch, got %d", host.fetchCalls)
	}
	if !o.HasActivePlacements() {
		t.Fatalf("expected active placements after settling on an eligible file")
	}
	if !bytes.Contains(written, []byte("\033_Ga=T,f=100,i=")) {
		t.Fatalf("expected a placement transmit sequence, got %q", written)
	}
}

func TestDisruptClearsActivePlacementsImmediately(t *testing.T) {
	t.Parallel()
	var written []byte
	o := newTestOverlay(&written)
	host := eligibleHost()
	host.old = encodePNG(t, 8, 8)
	host.oldOK = true
	host.newBytes = encodePNG(t, 16, 8)

	o, cmd := o.HandleSettle(SettleMsg{Seq: 0}, host)
	runCmd(t, cmd)
	if !o.HasActivePlacements() {
		t.Fatalf("precondition: expected placements to clear")
	}
	written = nil

	// Disrupt(false) clears without scheduling a settle tick, so the returned
	// command is a plain clear we can run synchronously.
	o, cmd = o.Disrupt(false)
	if o.HasActivePlacements() {
		t.Fatalf("expected active placements cleared from state")
	}
	runCmd(t, cmd)
	if !bytes.Contains(written, []byte("\033_Ga=d,d=I,i=1")) {
		t.Fatalf("expected a clear sequence for the active placement, got %q", written)
	}
}

func TestDisruptSchedulesSettleWhenEnabled(t *testing.T) {
	t.Parallel()
	var written []byte
	o := newTestOverlay(&written)

	before := o.SettleSeq()
	o, cmd := o.Disrupt(true)
	if o.SettleSeq() != before+1 {
		t.Fatalf("expected settle sequence to advance, got %d", o.SettleSeq())
	}
	if cmd == nil {
		t.Fatalf("expected a settle tick to be scheduled when enabled")
	}
}

func TestHandleSettleIgnoresStaleSeq(t *testing.T) {
	t.Parallel()
	var written []byte
	o := newTestOverlay(&written)
	o, _ = o.Disrupt(true) // settleSeq -> 1
	host := eligibleHost()

	o, cmd := o.HandleSettle(SettleMsg{Seq: 0}, host)
	if cmd != nil {
		t.Fatalf("expected no command for a stale settle")
	}
	if host.fetchCalls != 0 {
		t.Fatalf("expected no fetch for a stale settle, got %d", host.fetchCalls)
	}
	if o.HasActivePlacements() {
		t.Fatalf("expected no placement for a stale settle")
	}
}

func TestHandleSettleBailsWhileModalOpen(t *testing.T) {
	t.Parallel()
	var written []byte
	o := newTestOverlay(&written)
	host := eligibleHost()
	host.modalOpen = true

	o, _ = o.HandleSettle(SettleMsg{Seq: 0}, host)
	if host.fetchCalls != 0 {
		t.Fatalf("expected no blob fetch while a modal is open, got %d", host.fetchCalls)
	}
	if o.HasActivePlacements() {
		t.Fatalf("expected no placement while a modal is open")
	}
}

func TestHandleSettleMarksFallbackWhenBothSidesAbsent(t *testing.T) {
	t.Parallel()
	var written []byte
	o := newTestOverlay(&written)
	host := eligibleHost()
	host.oldOK = false
	host.newOK = false

	o, _ = o.HandleSettle(SettleMsg{Seq: 0}, host)
	if host.fetchCalls != 1 {
		t.Fatalf("expected one fetch attempt before falling back, got %d", host.fetchCalls)
	}
	if o.HasActivePlacements() {
		t.Fatalf("expected no placement when both sides are absent")
	}
	if o.FallbackPath() != host.path {
		t.Fatalf("expected fallback path %q, got %q", host.path, o.FallbackPath())
	}
}
