package commit

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
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/kittygraphics"

	"github.com/charmbracelet/x/ansi"
)

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

func supportedCapability() kittygraphics.Capability {
	return kittygraphics.Capability{Supported: true, PixelsPerCol: 10, PixelsPerRow: 20}
}

type commitImageDiffSpies struct {
	capability  kittygraphics.Capability
	old, new    []byte
	detectCalls int
	fetchCalls  int
	written     []byte
}

// setupCommitImageRepo creates a repo whose HEAD commit modifies image.png.
func setupCommitImageRepo(t *testing.T) string {
	t.Helper()
	repo := testutil.TempRepo(t)
	old := encodeTestPNG(t, 8, 8, color.RGBA{R: 255, A: 255})
	if err := os.WriteFile(repo+"/image.png", old, 0644); err != nil {
		t.Fatalf("write baseline image: %v", err)
	}
	testutil.MustGitExported(t, repo, "add", "image.png")
	testutil.MustGitExported(t, repo, "commit", "-m", "add image")

	updated := encodeTestPNG(t, 16, 8, color.RGBA{G: 255, A: 255})
	if err := os.WriteFile(repo+"/image.png", updated, 0644); err != nil {
		t.Fatalf("write updated image: %v", err)
	}
	testutil.MustGitExported(t, repo, "add", "image.png")
	testutil.MustGitExported(t, repo, "commit", "-m", "update image")
	return repo
}

func newCommitImageDiffModel(t *testing.T, repo string, capability kittygraphics.Capability) (Model, *commitImageDiffSpies) {
	t.Helper()
	settings := ui.Settings{UseNerdFontIcons: true, ImageDiffs: true}
	m := NewModel(repo, "HEAD", "", settings, keys.Manager{})
	m.ready = true
	m.width = 120
	m.height = 40

	old := encodeTestPNG(t, 8, 8, color.RGBA{R: 255, A: 255})
	newImg := encodeTestPNG(t, 16, 8, color.RGBA{G: 255, A: 255})
	spies := &commitImageDiffSpies{capability: capability, old: old, new: newImg}
	m.overlay = imagediff.NewOverlay(
		func(data []byte) { spies.written = append(spies.written, data...) },
		func() kittygraphics.Capability {
			spies.detectCalls++
			return spies.capability
		},
	)
	m.fetchImageDiffBlobs = func(ref string, file git.CommitFile) (oldBytes, newBytes []byte, oldOK, newOK bool) {
		spies.fetchCalls++
		return spies.old, spies.new, len(spies.old) > 0, len(spies.new) > 0
	}
	m.syncDiffViewport()
	return m, spies
}

func TestCommitImageDiffPlacesAfterSettleWhenVisible(t *testing.T) {
	t.Parallel()
	repo := setupCommitImageRepo(t)
	m, spies := newCommitImageDiffModel(t, repo, supportedCapability())

	// Injecting the screen origin (as the container does) is a disrupting event
	// that schedules a settle; firing it should place the overlay.
	m, cmd := m.WithScreenOrigin(0, 0, true)
	if cmd == nil {
		t.Fatalf("expected WithScreenOrigin to schedule a settle when becoming visible")
	}

	updated, settleCmd := m.Update(imagediff.SettleMsg{Seq: m.overlay.SettleSeq()})
	mm := updated.(Model)
	if settleCmd != nil {
		settleCmd()
	}

	if spies.fetchCalls == 0 {
		t.Fatalf("expected blob fetch once the panel settled on an eligible file")
	}
	if !mm.overlay.HasActivePlacements() {
		t.Fatalf("expected active placements after settling")
	}
	if !bytes.Contains(spies.written, []byte("\033_Ga=T,f=100,i=")) {
		t.Fatalf("expected a placement transmit sequence, got %q", spies.written)
	}

	view := ansi.Strip(mm.renderDiffPane(80, 20))
	if strings.Contains(view, "binary file") {
		t.Fatalf("expected reserved overlay space, not the binary summary line:\n%s", view)
	}
}

func TestCommitImageDiffStaysHiddenWhenNotVisible(t *testing.T) {
	t.Parallel()
	repo := setupCommitImageRepo(t)
	m, spies := newCommitImageDiffModel(t, repo, supportedCapability())

	// Detail not visible (collapsed split): a settle must not place anything.
	m, _ = m.WithScreenOrigin(40, 0, false)
	updated, _ := m.Update(imagediff.SettleMsg{Seq: m.overlay.SettleSeq()})
	mm := updated.(Model)

	if spies.fetchCalls != 0 {
		t.Fatalf("expected no blob fetch while the detail is not visible, got %d", spies.fetchCalls)
	}
	if mm.overlay.HasActivePlacements() {
		t.Fatalf("expected no placement while the detail is not visible")
	}
}

func TestCommitImageDiffUnsupportedTerminalFallsBack(t *testing.T) {
	t.Parallel()
	repo := setupCommitImageRepo(t)
	m, spies := newCommitImageDiffModel(t, repo, kittygraphics.Capability{Supported: false})

	m, _ = m.WithScreenOrigin(0, 0, true)
	updated, _ := m.Update(imagediff.SettleMsg{Seq: m.overlay.SettleSeq()})
	mm := updated.(Model)

	if spies.fetchCalls != 0 {
		t.Fatalf("expected no blob fetch on an unsupported terminal, got %d", spies.fetchCalls)
	}
	view := ansi.Strip(mm.renderDiffPane(80, 20))
	if !strings.Contains(view, "binary file") {
		t.Fatalf("expected the binary summary line on an unsupported terminal:\n%s", view)
	}
}
