package diff

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
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

	p := ParseUnifiedDiff(raw)
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
	p := ParseUnifiedDiff(raw)
	patch, err := BuildSingleLinePatch(p, 1)
	if err != nil {
		t.Fatalf("BuildSingleLinePatch: %v", err)
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
	p := ParseUnifiedDiff(raw)

	patch, err := BuildSingleLinePatch(p, 1) // +new-2
	if err != nil {
		t.Fatalf("BuildSingleLinePatch: %v", err)
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
	p := ParseUnifiedDiff(raw)

	patch, err := BuildHunkPatch(p, 0)
	if err != nil {
		t.Fatalf("BuildHunkPatch: %v", err)
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
	p := ParseUnifiedDiff(raw)

	patch, err := BuildLineRangePatch(p, 1, 3)
	if err != nil {
		t.Fatalf("BuildLineRangePatch: %v", err)
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
	p := ParseUnifiedDiff(raw)
	if len(p.Hunks) != 1 {
		t.Fatalf("expected one hunk, got %d", len(p.Hunks))
	}
	patch, err := BuildHunkPatch(p, 0)
	if err != nil {
		t.Fatalf("BuildHunkPatch: %v", err)
	}
	if err := git.ApplyPatchToIndex(repo, patch, false, false); err != nil {
		t.Fatalf("ApplyPatchToIndex failed: %v\npatch:\n%s", err, patch)
	}
}

func TestParseSymlinkDiffInfo(t *testing.T) {
	newSymlinkDiff := strings.Join([]string{
		"diff --git a/link b/link",
		"new file mode 120000",
		"index 0000000..abc1234",
		"--- /dev/null",
		"+++ b/link",
		"@@ -0,0 +1 @@",
		"+target/path",
		"\\ No newline at end of file",
	}, "\n") + "\n"

	modifiedSymlinkDiff := strings.Join([]string{
		"diff --git a/link b/link",
		"index abc1234..def5678 120000",
		"--- a/link",
		"+++ b/link",
		"@@ -1 +1 @@",
		"-old/target",
		"\\ No newline at end of file",
		"+new/target",
		"\\ No newline at end of file",
	}, "\n") + "\n"

	deletedSymlinkDiff := strings.Join([]string{
		"diff --git a/link b/link",
		"deleted file mode 120000",
		"index abc1234..0000000",
		"--- a/link",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-target/path",
		"\\ No newline at end of file",
	}, "\n") + "\n"

	regularToSymlinkDiff := strings.Join([]string{
		"diff --git a/myfile b/myfile",
		"deleted file mode 100644",
		"index 00cb5bc..0000000",
		"--- a/myfile",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-regular content",
		"diff --git a/myfile b/myfile",
		"new file mode 120000",
		"index 0000000..0d607a7",
		"--- /dev/null",
		"+++ b/myfile",
		"@@ -0,0 +1 @@",
		"+some/target",
		"\\ No newline at end of file",
	}, "\n") + "\n"

	symlinkToRegularDiff := strings.Join([]string{
		"diff --git a/link b/link",
		"deleted file mode 120000",
		"index 0d607a7..0000000",
		"--- a/link",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-some/target",
		"\\ No newline at end of file",
		"diff --git a/link b/link",
		"new file mode 100644",
		"index 0000000..b9a1f7f",
		"--- /dev/null",
		"+++ b/link",
		"@@ -0,0 +1 @@",
		"+now regular",
	}, "\n") + "\n"

	regularFileDiff := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"index 1111111..2222222 100644",
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1 +1 @@",
		"-old",
		"+new",
	}, "\n") + "\n"

	tests := []struct {
		name         string
		raw          string
		isSymlink    bool
		wasSymlink   bool
		isNowSymlink bool
		oldTarget    string
		newTarget    string
		summary      string
		titleLabel   string
	}{
		{
			name:         "new symlink",
			raw:          newSymlinkDiff,
			isSymlink:    true,
			wasSymlink:   false,
			isNowSymlink: true,
			oldTarget:    "",
			newTarget:    "target/path",
			summary:      "symlink -> target/path",
			titleLabel:   "[symlink]",
		},
		{
			name:         "modified symlink",
			raw:          modifiedSymlinkDiff,
			isSymlink:    true,
			wasSymlink:   true,
			isNowSymlink: true,
			oldTarget:    "old/target",
			newTarget:    "new/target",
			summary:      "symlink: old/target -> new/target",
			titleLabel:   "[symlink]",
		},
		{
			name:         "deleted symlink",
			raw:          deletedSymlinkDiff,
			isSymlink:    true,
			wasSymlink:   true,
			isNowSymlink: false,
			oldTarget:    "target/path",
			newTarget:    "",
			summary:      "symlink: target/path (removed)",
			titleLabel:   "[symlink]",
		},
		{
			name:         "regular file to symlink",
			raw:          regularToSymlinkDiff,
			isSymlink:    true,
			wasSymlink:   false,
			isNowSymlink: true,
			oldTarget:    "",
			newTarget:    "some/target",
			summary:      "regular file -> symlink (some/target)",
			titleLabel:   "[regular -> symlink]",
		},
		{
			name:         "symlink to regular file",
			raw:          symlinkToRegularDiff,
			isSymlink:    true,
			wasSymlink:   true,
			isNowSymlink: false,
			oldTarget:    "some/target",
			newTarget:    "",
			summary:      "symlink (some/target) -> regular file",
			titleLabel:   "[symlink -> regular]",
		},
		{
			name:      "regular file",
			raw:       regularFileDiff,
			isSymlink: false,
			summary:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ParseUnifiedDiff(tt.raw)
			si := ParseSymlinkDiffInfo(p)
			if si.IsSymlink != tt.isSymlink {
				t.Errorf("IsSymlink = %v, want %v", si.IsSymlink, tt.isSymlink)
			}
			if tt.isSymlink {
				if si.WasSymlink != tt.wasSymlink {
					t.Errorf("WasSymlink = %v, want %v", si.WasSymlink, tt.wasSymlink)
				}
				if si.IsNowSymlink != tt.isNowSymlink {
					t.Errorf("IsNowSymlink = %v, want %v", si.IsNowSymlink, tt.isNowSymlink)
				}
				if si.OldTarget != tt.oldTarget {
					t.Errorf("OldTarget = %q, want %q", si.OldTarget, tt.oldTarget)
				}
				if si.NewTarget != tt.newTarget {
					t.Errorf("NewTarget = %q, want %q", si.NewTarget, tt.newTarget)
				}
			}
			if got := si.Summary(); got != tt.summary {
				t.Errorf("summary = %q, want %q", got, tt.summary)
			}
			if got := si.TitleLabel(); got != tt.titleLabel {
				t.Errorf("titleLabel = %q, want %q", got, tt.titleLabel)
			}
		})
	}
}
