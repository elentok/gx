package explorer

import (
	"testing"

	"charm.land/bubbles/v2/viewport"

	"github.com/elentok/gx/ui/diff"
)

func TestRestoreViewportYOffset_ClampsToVisibleRange(t *testing.T) {
	vp := viewport.New(viewport.WithHeight(3))
	vp.SetContentLines([]string{"1", "2", "3", "4", "5"})

	RestoreViewportYOffset(&vp, 10)
	if got := vp.YOffset(); got != 2 {
		t.Fatalf("YOffset = %d, want 2", got)
	}

	RestoreViewportYOffset(&vp, -3)
	if got := vp.YOffset(); got != 0 {
		t.Fatalf("YOffset = %d, want 0", got)
	}
}

func TestHunkDisplayBounds_UsesExplicitDisplayRange(t *testing.T) {
	parsed := diff.ParseUnifiedDiff(sampleUnifiedDiff)

	start, end, ok := HunkDisplayBounds([][2]int{{4, 8}}, parsed, nil, 0)
	if !ok || start != 4 || end != 8 {
		t.Fatalf("HunkDisplayBounds = (%d, %d, %v), want (4, 8, true)", start, end, ok)
	}
}

func TestHunkDisplayBounds_FallsBackToDisplayMap(t *testing.T) {
	parsed := diff.ParseUnifiedDiff(sampleUnifiedDiff)

	displayToRaw := []int{-1, -1, 4, 5, 6}
	start, end, ok := HunkDisplayBounds(nil, parsed, displayToRaw, 0)
	if !ok || start != 2 || end != 4 {
		t.Fatalf("HunkDisplayBounds = (%d, %d, %v), want (2, 4, true)", start, end, ok)
	}
}

func TestVisualLineBounds_ClampsToChangedRange(t *testing.T) {
	start, end := VisualLineBounds(9, 1, 3)
	if start != 1 || end != 2 {
		t.Fatalf("VisualLineBounds = (%d, %d), want (1, 2)", start, end)
	}

	start, end = VisualLineBounds(-1, -1, 3)
	if start != 0 || end != 0 {
		t.Fatalf("VisualLineBounds = (%d, %d), want (0, 0)", start, end)
	}
}

const sampleUnifiedDiff = `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+two changed
 three
`
