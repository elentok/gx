package diffview

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/diffview/diffrender"
)

func TestBuildDiffBuffer_Unified(t *testing.T) {
	prev := NewDiffBuffer()
	data := BuildDiffBuffer(sampleSectionUnifiedDiff, "", prev, false)

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
	prev := NewDiffBuffer()
	color := " file:1: header\n  │ 1 │ one         │ 1 │ one         │\n  │ 2 │ two         │   │             │\n  │   │             │ 2 │ two changed │\n  │ 3 │ three       │ 3 │ three       │"
	data := BuildDiffBuffer(sampleSectionUnifiedDiff, color, prev, true)

	if !reflect.DeepEqual(data.ChangedDisplay, []int{2, 3}) {
		t.Fatalf("ChangedDisplay = %#v", data.ChangedDisplay)
	}
	if !reflect.DeepEqual(data.HunkDisplayRange, [][2]int{{0, 4}}) {
		t.Fatalf("HunkDisplayRange = %#v", data.HunkDisplayRange)
	}
}

func TestReflowDiffBuffer(t *testing.T) {
	data := DiffBuffer{
		Parsed:           diffcore.ParseUnifiedDiff(sampleSectionUnifiedDiff),
		BaseLines:        []string{"0123456789"},
		BaseLineKinds:    []diffrender.RowKind{diffrender.RowAdded},
		BaseDisplayToRaw: []int{6},
	}

	ReflowDiffBuffer(&data, 4, true)
	if len(data.ViewLines) < 2 {
		t.Fatalf("expected wrapped lines, got %#v", data.ViewLines)
	}
	if !reflect.DeepEqual(data.DisplayToRaw, []int{6, 6, 6}) {
		t.Fatalf("DisplayToRaw = %#v", data.DisplayToRaw)
	}
}
