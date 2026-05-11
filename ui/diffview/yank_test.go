package diffview

import (
	"reflect"
	"testing"
)

func TestFocusedYankBodyLineMode(t *testing.T) {
	section := BuildDiffBuffer(sampleSectionUnifiedDiff, "", NewDiffBuffer(), false)
	section.ActiveLine = 1

	got := FocusedYankBody(section, NavModeLine)
	want := []string{"+two changed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FocusedYankBody = %#v, want %#v", got, want)
	}
}

func TestFocusedYankBodyHunkMode(t *testing.T) {
	section := BuildDiffBuffer(sampleSectionUnifiedDiff, "", NewDiffBuffer(), false)

	got := FocusedYankBody(section, NavModeHunk)
	want := []string{" one", "-two", "+two changed", " three"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FocusedYankBody = %#v, want %#v", got, want)
	}
}

func TestFocusedLocationVisualLineMode(t *testing.T) {
	section := BuildDiffBuffer(sampleSectionUnifiedDiff, "", NewDiffBuffer(), false)
	section.ActiveLine = 1
	section.VisualActive = true
	section.VisualAnchor = 0

	got := FocusedLocation(section, NavModeLine)
	if got != "L2" {
		t.Fatalf("FocusedLocation = %q, want L2", got)
	}
}
