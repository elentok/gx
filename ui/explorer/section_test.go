package explorer

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/ui/diff"
)

func TestSplitLines(t *testing.T) {
	got := SplitLines("a\r\nb\r\n")
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitLines = %#v, want %#v", got, want)
	}

	if got := SplitLines(" \n"); got != nil {
		t.Fatalf("SplitLines blank = %#v, want nil", got)
	}
}

func TestIsDeltaSectionDivider(t *testing.T) {
	if !IsDeltaSectionDivider("────") {
		t.Fatal("expected box divider to match")
	}
	if !IsDeltaSectionDivider("----") {
		t.Fatal("expected ascii divider to match")
	}
	if IsDeltaSectionDivider("-- x --") {
		t.Fatal("expected mixed content not to match")
	}
}

func TestBuildSideBySideMapping(t *testing.T) {
	parsed := diff.ParseUnifiedDiff(sampleSectionUnifiedDiff)
	viewLines := []string{
		" file:1: header",
		"  │ 1 │ one         │ 1 │ one         │",
		"  │ 2 │ two         │   │             │",
		"  │   │             │ 2 │ two changed │",
		"  │ 3 │ three       │ 3 │ three       │",
	}

	got := BuildSideBySideMapping(parsed, viewLines)

	if !reflect.DeepEqual(got.DisplayToRaw, []int{-1, -1, 6, 7, -1}) {
		t.Fatalf("DisplayToRaw = %#v", got.DisplayToRaw)
	}
	if !reflect.DeepEqual(got.ChangedDisplay, []int{2, 3}) {
		t.Fatalf("ChangedDisplay = %#v", got.ChangedDisplay)
	}
	if !reflect.DeepEqual(got.HunkDisplayRange, [][2]int{{0, 4}}) {
		t.Fatalf("HunkDisplayRange = %#v", got.HunkDisplayRange)
	}
}

const sampleSectionUnifiedDiff = `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+two changed
 three
`
