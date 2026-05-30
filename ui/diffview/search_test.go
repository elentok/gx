package diffview

import (
	"testing"

	"charm.land/bubbles/v2/viewport"

	"github.com/elentok/gx/ui/search"
)

func TestApplyDiffSearchMatch(t *testing.T) {
	section := BuildDiffData(sampleSectionUnifiedDiff, "", NewDiffData(), false)
	vp := viewport.New(viewport.WithWidth(20), viewport.WithHeight(2))
	vp.SetContentLines(section.ViewLines)

	match := search.Match{DisplayIndex: 3, Index: 7}
	applyDiffSearchMatch(&section, &vp, match)
	if vp.YOffset() != 2 {
		t.Fatalf("YOffset = %d, want 2", vp.YOffset())
	}
	if section.ActiveLine != 1 {
		t.Fatalf("ActiveLine = %d, want 1", section.ActiveLine)
	}
	if section.ActiveHunk != 0 {
		t.Fatalf("ActiveHunk = %d, want 0", section.ActiveHunk)
	}
}

