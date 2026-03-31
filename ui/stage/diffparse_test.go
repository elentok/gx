package stage

import (
	"strings"
	"testing"

	"gx/git"
	"gx/testutil"
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

func TestBuildLineRangePatch_IncludesSelectedRange(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"index 1111111..2222222 100644",
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1,6 +1,6 @@",
		" keep-1",
		"-old-2",
		"+new-2",
		" keep-3",
		"-old-4",
		"+new-4",
		" keep-5",
	}, "\n") + "\n"
	p := parseUnifiedDiff(raw)

	patch, err := buildLineRangePatch(p, 1, 3)
	if err != nil {
		t.Fatalf("buildLineRangePatch: %v", err)
	}
	if !strings.Contains(patch, "+new-2") || !strings.Contains(patch, "+new-4") {
		t.Fatalf("expected both selected lines in patch:\n%s", patch)
	}
}

func TestBuildHunkPatch_ApplyToIndex_WithIndentedGoLines(t *testing.T) {
	repo := testutil.TempRepo(t)
	content := strings.Join([]string{
		"package p",
		"",
		"func f() {",
		"\ttm.Send(keyRune('y'))",
		"\twaitForText(t, tm, \"Force push?\", loadWait)",
		"",
		"\t// Confirm force push with 'y'.",
		"\ttm.Send(keyRune('y'))",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "a.go", content)
	testutil.CommitAll(t, repo, "baseline")

	updated := strings.Join([]string{
		"package p",
		"",
		"func f() {",
		"\ttm.Send(keyRune('y'))",
		"\twaitForText(t, tm, \"has diverged from the remote branch\", loadWait)",
		"",
		"\t// Choose force push.",
		"\ttm.Send(keyRune('2'))",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "a.go", updated)

	raw, err := git.DiffPath(repo, "a.go", false, 1)
	if err != nil {
		t.Fatalf("DiffPath: %v", err)
	}
	p := parseUnifiedDiff(raw)
	if len(p.Hunks) != 1 {
		t.Fatalf("expected one hunk, got %d", len(p.Hunks))
	}
	patch, err := buildHunkPatch(p, 0)
	if err != nil {
		t.Fatalf("buildHunkPatch: %v", err)
	}
	if err := git.ApplyPatchToIndex(repo, patch, false, false); err != nil {
		t.Fatalf("ApplyPatchToIndex failed: %v\npatch:\n%s", err, patch)
	}
}
