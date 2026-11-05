package commit

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/imagediff"
)

// diffPane owns the diff view, its context-line setting and the inline
// image-diff overlay (ADR 0010). Model keeps the layout-dependent methods: they
// need the pane geometry, which is derived from Model's size and files pane.
//
// The detail panel is composed into a split view and never learns its absolute
// screen position from lipgloss.Join*, so the container injects it through
// Model.WithScreenOrigin into screenCol/screenRow/screenVisible.
type diffPane struct {
	diffModel        diffview.Model
	diffContextLines int

	overlay             imagediff.Overlay
	screenCol           int
	screenRow           int
	screenVisible       bool
	fetchImageDiffBlobs func(ref string, file stage.CommitFile) (old, newBytes []byte, oldOK, newOK bool)
}

func (p diffPane) currentDiffContextLines() int {
	return min(max(p.diffContextLines, 1), 20)
}

// adjustDiffContextLines reports whether the clamped context size changed.
func (p *diffPane) adjustDiffContextLines(delta int) bool {
	next := min(max(p.currentDiffContextLines()+delta, 1), 20)
	if next == p.currentDiffContextLines() {
		return false
	}
	p.diffContextLines = next
	return true
}

func (p diffPane) title(focusDiff, nerdFont bool) string {
	ctx := fmt.Sprintf("Context: %d", p.currentDiffContextLines())
	return diffview.JoinDot(ctx, p.diffModel.StatusText(focusDiff), p.searchCounterText(focusDiff, nerdFont))
}

func (p diffPane) searchCounterText(focusDiff, nerdFont bool) string {
	s := p.diffModel.Search()
	if !s.HasQuery() || s.MatchesCount() == 0 || !focusDiff {
		return ""
	}
	icon := "⌕"
	if nerdFont {
		icon = ui.Icons(true).Search
	}
	return fmt.Sprintf("%s %d/%d", icon, s.Cursor()+1, s.MatchesCount())
}

func (p *diffPane) ensureActiveVisible() {
	p.diffModel.EnsureActiveVisible(p.diffModel.NavMode())
}

func (p diffPane) editorLineForCurrentSelection(focusDiff bool) int {
	if !focusDiff {
		return 0
	}
	diff := p.diffModel.DataRef()
	if p.diffModel.NavMode() == diffview.NavModeLine {
		if diff.ActiveLine < 0 || diff.ActiveLine >= len(diff.Parsed.Changed) {
			return 0
		}
		cl := diff.Parsed.Changed[diff.ActiveLine]
		if cl.NewLine > 0 {
			return cl.NewLine
		}
		return cl.OldLine
	}
	if diff.ActiveHunk < 0 || diff.ActiveHunk >= len(diff.Parsed.Hunks) {
		return 0
	}
	h := diff.Parsed.Hunks[diff.ActiveHunk]
	if h.NewStart > 0 {
		return h.NewStart
	}
	return h.OldStart
}

// isImageDiffEligible reports whether file is a candidate for inline image-diff
// rendering in the commit detail panel: extension allowlisted, config enabled,
// and the host terminal supports kitty graphics. Otherwise behavior is the plain
// "binary file" summary line.
func (p diffPane) isImageDiffEligible(imageDiffs bool, file stage.CommitFile) bool {
	if !imageDiffs {
		return false
	}
	if !imagediff.HasImageExtension(file.Path) {
		return false
	}
	return p.overlay.Capability().Supported
}

// binaryDiffLines returns the lines the diff pane renders for a binary file.
// When the selected file is an image-diff candidate (and its last settle didn't
// fall back), it reserves bodyH blank lines so bubbletea's layout accounts for
// the overlay area — the kitty graphics are placed as a side effect, never
// embedded here (ADR 0010). Otherwise it is the plain "binary file" summary.
func (p diffPane) binaryDiffLines(imageDiffs bool, file stage.CommitFile, hasFile bool, bodyH, innerW int) []string {
	if hasFile && p.isImageDiffEligible(imageDiffs, file) && p.overlay.FallbackPath() != file.Path {
		blank := strings.Repeat(" ", innerW)
		lines := make([]string, 0, bodyH)
		for range bodyH {
			lines = append(lines, blank)
		}
		return lines
	}
	return []string{ui.StyleMuted.Render("binary file")}
}
