package status

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/kittygraphics"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// encodeTestPNG returns a small valid PNG of the given size and fill color, so
// imagediff.Plan can decode it and produce a non-fallback layout.
func encodeTestPNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

// supportedCapability returns a Capability that DetectSupport would return for
// a directly-hosted kitty terminal with a typical cell pixel size.
func supportedCapability() kittygraphics.Capability {
	return kittygraphics.Capability{Supported: true, PixelsPerCol: 10, PixelsPerRow: 20}
}

// newImageDiffTestModel builds a status model over repo with a changed
// image.png, focused on its diff so renderDiffPane/Update exercise the
// image-diff lifecycle. Detection, blob fetching and terminal writes are
// stubbed so tests don't touch the real terminal or call out to git twice.
func newImageDiffTestModel(t *testing.T, repo string, settings ui.Settings, capability kittygraphics.Capability, old, new []byte) (*Model, *imageDiffSpies) {
	t.Helper()
	m := newTestModel(repo, settings, "")
	m.ready = true
	m.width = 120
	m.height = 30
	m.focus = focusFiletree

	spies := &imageDiffSpies{capability: capability, old: old, new: new}
	m.detectImageDiffCapability = func() kittygraphics.Capability {
		spies.detectCalls++
		return spies.capability
	}
	m.fetchImageDiffBlobs = func(file git.StageFileStatus, cached bool) (oldBytes, newBytes []byte, oldOK, newOK bool) {
		spies.fetchCalls++
		return spies.old, spies.new, len(spies.old) > 0, len(spies.new) > 0
	}
	m.writeImageDiffBytes = func(data []byte) {
		spies.written = append(spies.written, data...)
	}

	m.syncDiffViewports()
	return &m, spies
}

type imageDiffSpies struct {
	capability  kittygraphics.Capability
	old, new    []byte
	detectCalls int
	fetchCalls  int
	written     []byte
}

func settleImageDiff(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(imageDiffSettleMsg{seq: m.imageDiff.settleSeq})
	return updated.(Model)
}

func setupImageDiffRepo(t *testing.T) string {
	t.Helper()
	repo := testutil.TempRepo(t)
	old := encodeTestPNG(t, 8, 8, color.RGBA{R: 255, A: 255})
	if err := os.WriteFile(repo+"/a.png", old, 0644); err != nil {
		t.Fatalf("write baseline image: %v", err)
	}
	testutil.MustGitExported(t, repo, "add", "a.png")
	testutil.MustGitExported(t, repo, "commit", "-m", "add a.png")

	updated := encodeTestPNG(t, 16, 8, color.RGBA{G: 255, A: 255})
	if err := os.WriteFile(repo+"/a.png", updated, 0644); err != nil {
		t.Fatalf("write updated image: %v", err)
	}
	testutil.WriteFile(t, repo, "0-before.txt", "hello\n")
	testutil.WriteFile(t, repo, "z-after.txt", "hello\n")
	return repo
}

func TestImageDiffSelectionChangeClearsActivePlacementImmediately(t *testing.T) {
	t.Parallel()
	repo := setupImageDiffRepo(t)
	old := encodeTestPNG(t, 8, 8, color.RGBA{R: 255, A: 255})
	new := encodeTestPNG(t, 16, 8, color.RGBA{G: 255, A: 255})
	settings := DefaultSettings()
	settings.ImageDiffs = true

	m, spies := newImageDiffTestModel(t, repo, settings, supportedCapability(), old, new)
	m.imageDiff.activeIDs = []uint32{1, 2}
	m.imageDiff.capability = spies.capability
	m.imageDiff.capabilityDetected = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatalf("expected a clear command to be emitted immediately on selection change")
	}
	runStatusCmd(t, mm, cmd)

	if !bytes.Contains(spies.written, []byte("\033_Ga=d,d=I,i=1")) || !bytes.Contains(spies.written, []byte("\033_Ga=d,d=I,i=2")) {
		t.Fatalf("expected clear sequences for both active placements, got %q", spies.written)
	}
	if len(mm.imageDiff.activeIDs) != 0 {
		t.Fatalf("expected active placements to be cleared from state, got %v", mm.imageDiff.activeIDs)
	}
}

func TestImageDiffSettlesAndPlacesAfterDebounce(t *testing.T) {
	t.Parallel()
	repo := setupImageDiffRepo(t)
	old := encodeTestPNG(t, 8, 8, color.RGBA{R: 255, A: 255})
	new := encodeTestPNG(t, 16, 8, color.RGBA{G: 255, A: 255})
	settings := DefaultSettings()
	settings.ImageDiffs = true

	m, spies := newImageDiffTestModel(t, repo, settings, supportedCapability(), old, new)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatalf("expected selection change to schedule a settle debounce")
	}
	mm = runStatusCmd(t, mm, cmd)

	if spies.fetchCalls == 0 {
		t.Fatalf("expected blob fetch once the model settled on an eligible file")
	}
	if len(mm.imageDiff.activeIDs) == 0 {
		t.Fatalf("expected active placement IDs to be recorded after settling")
	}
	if !bytes.Contains(spies.written, []byte("\033_Ga=T,f=100,i=")) {
		t.Fatalf("expected a placement transmit sequence to be written, got %q", spies.written)
	}
}

func TestImageDiffNoPlacementWhenSelectionMovesAwayBeforeSettling(t *testing.T) {
	t.Parallel()
	repo := setupImageDiffRepo(t)
	old := encodeTestPNG(t, 8, 8, color.RGBA{R: 255, A: 255})
	new := encodeTestPNG(t, 16, 8, color.RGBA{G: 255, A: 255})
	settings := DefaultSettings()
	settings.ImageDiffs = true

	m, spies := newImageDiffTestModel(t, repo, settings, supportedCapability(), old, new)

	// "0-before.txt" is selected initially; move onto the image, then past it
	// to "z-after.txt" before the first debounce fires — only the stale settle
	// for the (now-superseded) image selection arrives.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	mm := updated.(Model)
	staleSeq := mm.imageDiff.settleSeq

	updated, _ = mm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	mm = updated.(Model)

	updated, _ = mm.Update(imageDiffSettleMsg{seq: staleSeq})
	mm = updated.(Model)

	if spies.fetchCalls != 0 {
		t.Fatalf("expected no fetch for a settle message superseded by a later selection change")
	}
	if len(mm.imageDiff.activeIDs) != 0 {
		t.Fatalf("expected no placement when settle is stale, got %v", mm.imageDiff.activeIDs)
	}
}

func TestImageDiffNotPlacedWhileModalIsOpen(t *testing.T) {
	t.Parallel()
	repo := setupImageDiffRepo(t)
	old := encodeTestPNG(t, 8, 8, color.RGBA{R: 255, A: 255})
	new := encodeTestPNG(t, 16, 8, color.RGBA{G: 255, A: 255})
	settings := DefaultSettings()
	settings.ImageDiffs = true

	m, spies := newImageDiffTestModel(t, repo, settings, supportedCapability(), old, new)

	// Select the image, then open a modal before the debounce settles — a
	// placement here would paint over the modal at the terminal's graphics
	// layer (ADR 0010), so the settle must bail out without placing.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	mm := updated.(Model)
	mm.confirmOpen = true

	updated, _ = mm.Update(imageDiffSettleMsg{seq: mm.imageDiff.settleSeq})
	mm = updated.(Model)

	if spies.fetchCalls != 0 {
		t.Fatalf("expected no blob fetch while a modal is open, got %d calls", spies.fetchCalls)
	}
	if len(mm.imageDiff.activeIDs) != 0 {
		t.Fatalf("expected no placement while a modal is open, got %v", mm.imageDiff.activeIDs)
	}

	// Closing the modal (esc -> handleConfirmKey sets confirmOpen = false)
	// re-marks imageDiff dirty via the ModalOpen() transition check, scheduling
	// a fresh settle that places the overlay now that it's safe to.
	updated, cmd := mm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mm = updated.(Model)
	if mm.confirmOpen {
		t.Fatalf("expected esc to close the confirm modal")
	}
	mm = runStatusCmd(t, mm, cmd)

	if spies.fetchCalls == 0 {
		t.Fatalf("expected a blob fetch once the modal closed and the model resettled")
	}
	if len(mm.imageDiff.activeIDs) == 0 {
		t.Fatalf("expected the overlay to be placed once the modal closed")
	}
}

func TestImageDiffsConfigDisabledShortCircuitsToBinarySummary(t *testing.T) {
	t.Parallel()
	repo := setupImageDiffRepo(t)
	old := encodeTestPNG(t, 8, 8, color.RGBA{R: 255, A: 255})
	new := encodeTestPNG(t, 16, 8, color.RGBA{G: 255, A: 255})
	settings := DefaultSettings()
	settings.ImageDiffs = false

	m, spies := newImageDiffTestModel(t, repo, settings, supportedCapability(), old, new)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	mm := updated.(Model)
	mm = runStatusCmd(t, mm, cmd)
	mm = settleImageDiff(t, mm)

	view := ansi.Strip(mm.renderDiffPane(80, 14))
	if !strings.Contains(view, "binary file") {
		t.Fatalf("expected binary summary when image-diffs is disabled, got:\n%s", view)
	}
	if spies.detectCalls != 0 {
		t.Fatalf("expected no capability detection when image-diffs is disabled, got %d calls", spies.detectCalls)
	}
	if spies.fetchCalls != 0 {
		t.Fatalf("expected no blob fetch when image-diffs is disabled, got %d calls", spies.fetchCalls)
	}
}

func TestImageDiffUnsupportedTerminalShortCircuitsToBinarySummary(t *testing.T) {
	t.Parallel()
	repo := setupImageDiffRepo(t)
	old := encodeTestPNG(t, 8, 8, color.RGBA{R: 255, A: 255})
	new := encodeTestPNG(t, 16, 8, color.RGBA{G: 255, A: 255})
	settings := DefaultSettings()
	settings.ImageDiffs = true

	unsupported := kittygraphics.Capability{Supported: false}
	m, spies := newImageDiffTestModel(t, repo, settings, unsupported, old, new)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	mm := updated.(Model)
	mm = runStatusCmd(t, mm, cmd)
	mm = settleImageDiff(t, mm)

	view := ansi.Strip(mm.renderDiffPane(80, 14))
	if !strings.Contains(view, "binary file") {
		t.Fatalf("expected binary summary on an unsupported terminal, got:\n%s", view)
	}
	if spies.fetchCalls != 0 {
		t.Fatalf("expected no blob fetch on an unsupported terminal, got %d calls", spies.fetchCalls)
	}
}

func TestImageDiffPlanFallbackShowsBinarySummary(t *testing.T) {
	t.Parallel()
	repo := setupImageDiffRepo(t)
	// Non-image bytes: imagediff.Plan will fail to decode and return a fallback plan.
	corrupt := []byte("not an image")
	settings := DefaultSettings()
	settings.ImageDiffs = true

	m, spies := newImageDiffTestModel(t, repo, settings, supportedCapability(), corrupt, corrupt)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	mm := updated.(Model)
	mm = runStatusCmd(t, mm, cmd)
	mm = settleImageDiff(t, mm)

	if len(mm.imageDiff.activeIDs) != 0 {
		t.Fatalf("expected no placement for a fallback plan, got %v", mm.imageDiff.activeIDs)
	}
	if spies.fetchCalls == 0 {
		t.Fatalf("expected a blob fetch attempt before falling back")
	}

	view := ansi.Strip(mm.renderDiffPane(80, 14))
	if !strings.Contains(view, "binary file") {
		t.Fatalf("expected binary summary when the layout plan falls back, got:\n%s", view)
	}
}
