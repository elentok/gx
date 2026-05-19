package diffrender

import (
	"strings"
	"testing"

	"github.com/elentok/gx/ui/diffview/diffcore"
)

func TestParseSymlinkDiffInfo_NewSymlink(t *testing.T) {
	raw := "diff --git a/link b/link\nnew file mode 120000\nindex 0000000..abc1234\n--- /dev/null\n+++ b/link\n@@ -0,0 +1 @@\n+../target\n"
	parsed := diffcore.ParseUnifiedDiff(raw)
	info := ParseSymlinkDiffInfo(parsed)
	if !info.IsSymlink || !info.IsNowSymlink || info.WasSymlink {
		t.Errorf("unexpected symlink flags: %+v", info)
	}
	if info.NewTarget != "../target" {
		t.Errorf("NewTarget = %q, want %q", info.NewTarget, "../target")
	}
}

func TestParseSymlinkDiffInfo_DeletedSymlink(t *testing.T) {
	raw := "diff --git a/link b/link\ndeleted file mode 120000\nindex abc1234..0000000\n--- a/link\n+++ /dev/null\n@@ -1 +0,0 @@\n-../target\n"
	parsed := diffcore.ParseUnifiedDiff(raw)
	info := ParseSymlinkDiffInfo(parsed)
	if !info.IsSymlink || !info.WasSymlink || info.IsNowSymlink {
		t.Errorf("unexpected symlink flags: %+v", info)
	}
	if info.OldTarget != "../target" {
		t.Errorf("OldTarget = %q, want %q", info.OldTarget, "../target")
	}
}

func TestParseSymlinkDiffInfo_NonSymlink(t *testing.T) {
	raw := "diff --git a/file.txt b/file.txt\nnew file mode 100644\n"
	parsed := diffcore.ParseUnifiedDiff(raw)
	info := ParseSymlinkDiffInfo(parsed)
	if info.IsSymlink {
		t.Error("expected IsSymlink=false for non-symlink diff")
	}
}

func TestCleanHunkHeader(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"@@ -1,2 +1,2 @@ func foo()", "func foo()"},
		{"@@ -1,2 +1,2 @@", "hunk"},
		{"no at-signs", "no at-signs"},
		{"@@ only one", "@@ only one"}, // single @@ → trimmed as-is
	}
	for _, tt := range tests {
		got := CleanHunkHeader(tt.in)
		if got != tt.want {
			t.Errorf("CleanHunkHeader(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStripUnifiedVisibleMarker(t *testing.T) {
	// marker at position 0
	got := StripUnifiedVisibleMarker("+hello", '+')
	if len(got) == 0 {
		t.Error("expected non-empty result")
	}
	if got[0] == '+' {
		t.Errorf("marker should have been replaced, got %q", got)
	}

	// no marker present — unchanged
	line := "  hello"
	got = StripUnifiedVisibleMarker(line, '+')
	if got != line {
		t.Errorf("expected unchanged line, got %q", got)
	}

	// empty string — unchanged
	got = StripUnifiedVisibleMarker("", '+')
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestSymlinkDiffInfoTitleLabel(t *testing.T) {
	tests := []struct {
		info SymlinkDiffInfo
		want string
	}{
		{SymlinkDiffInfo{WasSymlink: true, IsNowSymlink: true}, "[symlink]"},
		{SymlinkDiffInfo{WasSymlink: false, IsNowSymlink: true, TypeChange: true}, "[regular -> symlink]"},
		{SymlinkDiffInfo{WasSymlink: false, IsNowSymlink: true, TypeChange: false}, "[symlink]"},
		{SymlinkDiffInfo{WasSymlink: true, IsNowSymlink: false, TypeChange: true}, "[symlink -> regular]"},
		{SymlinkDiffInfo{WasSymlink: true, IsNowSymlink: false, TypeChange: false}, "[symlink]"},
		{SymlinkDiffInfo{}, ""},
	}
	for _, tt := range tests {
		if got := tt.info.TitleLabel(); got != tt.want {
			t.Errorf("TitleLabel(%+v) = %q, want %q", tt.info, got, tt.want)
		}
	}
}

func TestSymlinkDiffInfoSummary(t *testing.T) {
	// both symlinks with targets
	si := SymlinkDiffInfo{WasSymlink: true, IsNowSymlink: true, OldTarget: "old", NewTarget: "new"}
	if got := si.Summary(); got != "symlink: old -> new" {
		t.Errorf("Summary() = %q", got)
	}

	// new symlink only
	si = SymlinkDiffInfo{WasSymlink: true, IsNowSymlink: true, NewTarget: "new"}
	if got := si.Summary(); got != "symlink -> new" {
		t.Errorf("Summary() = %q", got)
	}

	// removed symlink
	si = SymlinkDiffInfo{WasSymlink: true, IsNowSymlink: true, OldTarget: "old"}
	if got := si.Summary(); got != "symlink: old (removed)" {
		t.Errorf("Summary() = %q", got)
	}

	// type change to symlink
	si = SymlinkDiffInfo{WasSymlink: false, IsNowSymlink: true, TypeChange: true, NewTarget: "t"}
	if got := si.Summary(); got != "regular file -> symlink (t)" {
		t.Errorf("Summary() = %q", got)
	}

	// type change from symlink
	si = SymlinkDiffInfo{WasSymlink: true, IsNowSymlink: false, TypeChange: true, OldTarget: "t"}
	if got := si.Summary(); got != "symlink (t) -> regular file" {
		t.Errorf("Summary() = %q", got)
	}
}

func TestSanitizeANSIInline(t *testing.T) {
	// keeps color escape but strips cursor movement
	input := "\x1b[32mgreen\x1b[0m"
	got := SanitizeANSIInline(input)
	if !strings.Contains(got, "green") {
		t.Errorf("expected 'green' in output, got %q", got)
	}

	// strips control characters except ESC
	got = SanitizeANSIInline("hel\x01lo")
	if strings.Contains(got, "\x01") {
		t.Errorf("expected control char stripped, got %q", got)
	}

	// replaces tabs
	got = SanitizeANSIInline("a\tb")
	if !strings.Contains(got, "    ") {
		t.Errorf("expected tab replaced with spaces, got %q", got)
	}
}

func TestWrapANSI(t *testing.T) {
	// short string — no wrap
	got := WrapANSI("hello", 80)
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("expected single part, got %v", got)
	}

	// zero width — no wrap
	got = WrapANSI("hello", 0)
	if len(got) != 1 {
		t.Errorf("expected single part for zero width, got %v", got)
	}
}

func TestDiffBodyPadding(t *testing.T) {
	// zero width → empty
	if got := DiffBodyPadding(RowAdded, 0); got != "" {
		t.Errorf("expected empty for width=0, got %q", got)
	}

	// positive width → non-empty
	if got := DiffBodyPadding(RowPlain, 4); got == "" {
		t.Error("expected non-empty padding for width=4")
	}
}

func TestBuildDisplayBaseLines(t *testing.T) {
	raw := `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1,2 +1,2 @@
-old
+new
`

	parsed := diffcore.ParseUnifiedDiff(raw)
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
