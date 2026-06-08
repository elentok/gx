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
	"github.com/elentok/gx/ui/imagediff"
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
	m.overlay = imagediff.NewOverlay(
		func(data []byte) { spies.written = append(spies.written, data...) },
		func() kittygraphics.Capability {
			spies.detectCalls++
			return spies.capability
		},
	)
	m.fetchImageDiffBlobs = func(file git.StageFileStatus, cached bool) (oldBytes, newBytes []byte, oldOK, newOK bool) {
		spies.fetchCalls++
		return spies.old, spies.new, len(spies.old) > 0, len(spies.new) > 0
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
	updated, _ := m.Update(imagediff.SettleMsg{Seq: m.overlay.SettleSeq()})
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

	if mm.overlay.HasActivePlacements() {
		t.Fatalf("expected no placement for a fallback plan")
	}
	if spies.fetchCalls == 0 {
		t.Fatalf("expected a blob fetch attempt before falling back")
	}

	view := ansi.Strip(mm.renderDiffPane(80, 14))
	if !strings.Contains(view, "binary file") {
		t.Fatalf("expected binary summary when the layout plan falls back, got:\n%s", view)
	}
}

// TestImageDiffEndToEndPlacesAfterSettle exercises the full status wiring: a
// selection change schedules a settle, and the settle places the overlay and
// records active placements (the controller-level lifecycle is covered in
// ui/imagediff).
func TestImageDiffEndToEndPlacesAfterSettle(t *testing.T) {
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
	if !mm.overlay.HasActivePlacements() {
		t.Fatalf("expected active placements to be recorded after settling")
	}
	if !bytes.Contains(spies.written, []byte("\033_Ga=T,f=100,i=")) {
		t.Fatalf("expected a placement transmit sequence to be written, got %q", spies.written)
	}
}
