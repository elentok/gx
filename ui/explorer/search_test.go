package explorer

import (
	"testing"

	"charm.land/bubbles/v2/viewport"

	"github.com/elentok/gx/ui/diff"
)

func TestComputeDiffSearchMatches(t *testing.T) {
	matches := ComputeDiffSearchMatches(
		[]string{"one", "Two", "three two"},
		[]int{-1, 7, 9},
		"two",
	)
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(matches))
	}
	if matches[0].DisplayIndex != 1 || matches[1].DisplayIndex != 2 {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}

func TestApplyDiffSearchMatch(t *testing.T) {
	section := BuildSectionData(sampleSectionUnifiedDiff, "", NewSectionData(), false)
	vp := viewport.New(viewport.WithWidth(20), viewport.WithHeight(2))
	vp.SetContentLines(section.ViewLines)

	match := DiffSearchMatch{DisplayIndex: 3, RawIndex: 7}
	ApplyDiffSearchMatch(&section, &vp, match)
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

func TestCurrentDiffSearchMatchIndex(t *testing.T) {
	section := SectionData{
		Parsed:     diff.ParseUnifiedDiff(sampleSectionUnifiedDiff),
		ActiveLine: 1,
	}
	matches := []DiffSearchMatch{
		{DisplayIndex: 2, RawIndex: 6},
		{DisplayIndex: 3, RawIndex: 7},
	}
	got := CurrentDiffSearchMatchIndex(section, matches, NavLine)
	if got != 1 {
		t.Fatalf("CurrentDiffSearchMatchIndex = %d, want 1", got)
	}
}
