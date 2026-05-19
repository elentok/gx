package diffview

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/diffview/diffrender"
)

func TestBuildDiffBuffer_Unified(t *testing.T) {
	prev := NewDiffData()
	data := BuildDiffData(sampleSectionUnifiedDiff, "", prev, false)

	if len(data.Parsed.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(data.Parsed.Hunks))
	}
	if data.ActiveHunk != 0 || data.ActiveLine != 0 {
		t.Fatalf("active = (%d,%d), want (0,0)", data.ActiveHunk, data.ActiveLine)
	}
	if len(data.BaseLines) == 0 || len(data.ViewLines) == 0 {
		t.Fatal("expected built display lines")
	}
}

func TestBuildDiffBuffer_SideBySide(t *testing.T) {
	prev := NewDiffData()
	color := " file:1: header\n  │ 1 │ one         │ 1 │ one         │\n  │ 2 │ two         │   │             │\n  │   │             │ 2 │ two changed │\n  │ 3 │ three       │ 3 │ three       │"
	data := BuildDiffData(sampleSectionUnifiedDiff, color, prev, true)

	if !reflect.DeepEqual(data.ChangedDisplay, []int{2, 3}) {
		t.Fatalf("ChangedDisplay = %#v", data.ChangedDisplay)
	}
	if !reflect.DeepEqual(data.HunkDisplayRange, [][2]int{{0, 4}}) {
		t.Fatalf("HunkDisplayRange = %#v", data.HunkDisplayRange)
	}
}

func TestHunkDisplayBounds(t *testing.T) {
	prev := NewDiffData()
	data := BuildDiffData(sampleSectionUnifiedDiff, "", prev, false)

	// valid hunk index
	start, end, ok := data.HunkDisplayBounds(0)
	if !ok {
		t.Fatal("expected ok=true for hunk 0")
	}
	if start > end {
		t.Errorf("start(%d) > end(%d)", start, end)
	}

	// out of range
	_, _, ok = data.HunkDisplayBounds(-1)
	if ok {
		t.Error("expected ok=false for hunk -1")
	}
	_, _, ok = data.HunkDisplayBounds(999)
	if ok {
		t.Error("expected ok=false for hunk 999")
	}

	// with HunkDisplayRange populated (side-by-side path)
	color := " file:1: header\n  │ 1 │ one         │ 1 │ one         │\n  │ 2 │ two         │   │             │\n  │   │             │ 2 │ two changed │\n  │ 3 │ three       │ 3 │ three       │"
	sbsData := BuildDiffData(sampleSectionUnifiedDiff, color, prev, true)
	start, end, ok = sbsData.HunkDisplayBounds(0)
	if !ok {
		t.Fatal("expected ok=true for sbs hunk 0")
	}
	if start > end {
		t.Errorf("sbs: start(%d) > end(%d)", start, end)
	}
}

func TestVisualLineBounds(t *testing.T) {
	prev := NewDiffData()
	data := BuildDiffData(sampleSectionUnifiedDiff, "", prev, false)

	// no visual selection: anchor=-1, active=0 → both clamp to valid
	data.VisualAnchor = -1
	data.ActiveLine = 0
	s, e := data.VisualLineBounds()
	if s < 0 || e < 0 {
		t.Errorf("bounds should be non-negative, got (%d, %d)", s, e)
	}

	// anchor > active → they swap
	data.VisualAnchor = 2
	data.ActiveLine = 0
	s, e = data.VisualLineBounds()
	if s > e {
		t.Errorf("expected s <= e after swap, got (%d, %d)", s, e)
	}
}

func TestReflowDiffData(t *testing.T) {
	data := DiffData{
		Parsed:           diffcore.ParseUnifiedDiff(sampleSectionUnifiedDiff),
		BaseLines:        []string{"0123456789"},
		BaseLineKinds:    []diffrender.RowKind{diffrender.RowAdded},
		BaseDisplayToRaw: []int{6},
	}

	reflowDiffData(&data, 4, true)
	if len(data.ViewLines) < 2 {
		t.Fatalf("expected wrapped lines, got %#v", data.ViewLines)
	}
	if !reflect.DeepEqual(data.DisplayToRaw, []int{6, 6, 6}) {
		t.Fatalf("DisplayToRaw = %#v", data.DisplayToRaw)
	}
}
