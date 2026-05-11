package diffview

import (
	"testing"

	"charm.land/bubbles/v2/viewport"
)

func TestMoveActiveLine(t *testing.T) {
	section := BuildDiffBuffer(sampleSectionUnifiedDiff, "", NewDiffBuffer(), false)
	vp := viewport.New(viewport.WithWidth(20), viewport.WithHeight(2))
	vp.SetContentLines(section.ViewLines)

	changed := MoveActive(&section, &vp, NavModeLine, 1, false)
	if !changed {
		t.Fatal("expected line movement")
	}
	if section.ActiveLine != 1 {
		t.Fatalf("ActiveLine = %d, want 1", section.ActiveLine)
	}
}

func TestMoveActiveHunkCanScrollViewport(t *testing.T) {
	section := DiffBuffer{
		Parsed:           BuildDiffBuffer(sampleSectionUnifiedDiff, "", NewDiffBuffer(), false).Parsed,
		HunkDisplayRange: [][2]int{{4, 8}},
		ActiveHunk:       0,
	}
	vp := viewport.New(viewport.WithWidth(20), viewport.WithHeight(2))
	vp.SetContentLines([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8"})

	changed := MoveActive(&section, &vp, NavModeHunk, 1, true)
	if changed {
		t.Fatal("expected viewport scroll before hunk movement")
	}
	if vp.YOffset() != 1 {
		t.Fatalf("YOffset = %d, want 1", vp.YOffset())
	}
}

func TestJumpTopAndBottom(t *testing.T) {
	section := BuildDiffBuffer(sampleSectionUnifiedDiff, "", NewDiffBuffer(), false)
	vp := viewport.New(viewport.WithWidth(20), viewport.WithHeight(2))
	vp.SetContentLines(section.ViewLines)

	if !JumpBottom(&section, &vp, NavModeLine) {
		t.Fatal("expected JumpBottom to succeed")
	}
	if section.ActiveLine != len(section.Parsed.Changed)-1 {
		t.Fatalf("ActiveLine = %d", section.ActiveLine)
	}

	if !JumpTop(&section, &vp, NavModeLine) {
		t.Fatal("expected JumpTop to succeed")
	}
	if section.ActiveLine != 0 {
		t.Fatalf("ActiveLine = %d, want 0", section.ActiveLine)
	}
}
