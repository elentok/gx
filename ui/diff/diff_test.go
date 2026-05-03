package diff

import "testing"

func TestBuildDisplayBaseLines(t *testing.T) {
	raw := `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1,2 +1,2 @@
-old
+new
`

	parsed := ParseUnifiedDiff(raw)
	if len(parsed.Hunks) != 1 {
		t.Fatalf("expected one hunk, got %#v", parsed.Hunks)
	}

	lines, kinds, displayToRaw := BuildDisplayBaseLines(parsed, nil)
	if len(lines) == 0 || len(kinds) == 0 || len(displayToRaw) == 0 {
		t.Fatalf("expected rendered lines, got lines=%v kinds=%v displayToRaw=%v", lines, kinds, displayToRaw)
	}
	if kinds[len(kinds)-2] != RowRemoved || kinds[len(kinds)-1] != RowAdded {
		t.Fatalf("unexpected row kinds: %v", kinds)
	}
}
