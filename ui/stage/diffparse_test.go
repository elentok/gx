package stage

import (
	"strings"
	"testing"
)

func TestParseUnifiedDiff_TracksHunksAndChangedLines(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"index 1111111..2222222 100644",
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1,3 +1,3 @@",
		" one",
		"-two",
		"+two changed",
		" three",
	}, "\n") + "\n"

	p := parseUnifiedDiff(raw)
	if len(p.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(p.Hunks))
	}
	if len(p.Changed) != 2 {
		t.Fatalf("changed = %d, want 2", len(p.Changed))
	}
	if p.Changed[0].Prefix != '-' || p.Changed[1].Prefix != '+' {
		t.Fatalf("unexpected changed prefixes: %#v", p.Changed)
	}
}

func TestBuildSingleLinePatch(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"index 1111111..2222222 100644",
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1,2 +1,2 @@",
		"-old",
		"+new",
	}, "\n") + "\n"
	p := parseUnifiedDiff(raw)
	patch, err := buildSingleLinePatch(p, 1)
	if err != nil {
		t.Fatalf("buildSingleLinePatch: %v", err)
	}
	if !strings.Contains(patch, "@@ -") {
		t.Fatalf("unexpected hunk header in patch:\n%s", patch)
	}
	if !strings.Contains(patch, "+new") {
		t.Fatalf("patch missing selected line:\n%s", patch)
	}
}

func TestBuildSingleLinePatch_DoesNotIncludeNonContiguousContext(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"index 1111111..2222222 100644",
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1,7 +1,7 @@",
		" keep-1",
		"-old-2",
		"+new-2",
		" keep-3",
		"-old-4",
		"+new-4",
		" keep-5",
	}, "\n") + "\n"
	p := parseUnifiedDiff(raw)

	patch, err := buildSingleLinePatch(p, 1) // +new-2
	if err != nil {
		t.Fatalf("buildSingleLinePatch: %v", err)
	}

	if strings.Contains(patch, "keep-5") || strings.Contains(patch, "old-4") || strings.Contains(patch, "new-4") {
		t.Fatalf("patch includes unrelated non-contiguous lines:\n%s", patch)
	}
	if !strings.Contains(patch, "+new-2") {
		t.Fatalf("patch missing selected line:\n%s", patch)
	}
}

func TestBuildHunkPatch_PreservesFullFileHeader(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"new file mode 100644",
		"index 0000000..1111111",
		"--- /dev/null",
		"+++ b/a.txt",
		"@@ -0,0 +1,2 @@",
		"+one",
		"+two",
	}, "\n") + "\n"
	p := parseUnifiedDiff(raw)

	patch, err := buildHunkPatch(p, 0)
	if err != nil {
		t.Fatalf("buildHunkPatch: %v", err)
	}
	if !strings.Contains(patch, "new file mode 100644") || !strings.Contains(patch, "index 0000000..1111111") || !strings.Contains(patch, "--- /dev/null") {
		t.Fatalf("expected patch to preserve file header metadata:\n%s", patch)
	}
}
