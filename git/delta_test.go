package git

import (
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestDeltaAvailable(t *testing.T) {
	// Just call it — result depends on whether delta is installed on the host.
	_ = DeltaAvailable()
}

func TestColorizeDiff_EmptyRaw(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	out, err := ColorizeDiff(dir, "file.go", "   ", false, false, 80, 3)
	if err != nil {
		t.Errorf("ColorizeDiff with empty raw: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output for empty raw, got %q", out)
	}
}

func TestColorizeUntrackedDiff_EmptyRaw(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	out, err := ColorizeUntrackedDiff(dir, "file.go", "  ", false, 80, 3)
	if err != nil {
		t.Errorf("ColorizeUntrackedDiff with empty raw: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output for empty raw, got %q", out)
	}
}

func TestColorizeDiff_FallbackWithoutDelta(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "file.txt", "hello\n")
	testutil.MustGitExported(t, dir, "add", "file.txt")
	testutil.MustGitExported(t, dir, "commit", "-m", "init")
	testutil.WriteFile(t, dir, "file.txt", "world\n")
	testutil.MustGitExported(t, dir, "add", "file.txt")

	// Get the raw diff
	raw, _, err := run(dir, []string{"diff", "--no-color", "--cached", "--", "file.txt"})
	if err != nil || raw == "" {
		t.Skip("could not get raw diff")
	}

	// ColorizeDiff should either use delta or fall back to git color
	out, err := ColorizeDiff(dir, "file.txt", raw, true, false, 80, 3)
	if err != nil {
		t.Errorf("ColorizeDiff fallback: %v", err)
	}
	_ = out
}

func TestColorizeDiff_WithContent(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "col.txt", "original\n")
	testutil.MustGitExported(t, dir, "add", "col.txt")
	testutil.MustGitExported(t, dir, "commit", "-m", "init")
	testutil.WriteFile(t, dir, "col.txt", "modified\n")
	testutil.MustGitExported(t, dir, "add", "col.txt")

	raw, _, err := run(dir, []string{"diff", "--no-color", "--cached", "--", "col.txt"})
	if err != nil || raw == "" {
		t.Skip("could not get raw diff")
	}

	out, err := ColorizeDiff(dir, "col.txt", raw, true, false, 80, 3)
	if err != nil {
		t.Fatalf("ColorizeDiff: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty colorized output")
	}
}

func TestColorizeUntrackedDiff_WithContent(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	path := dir + "/untracked.txt"
	testutil.WriteFile(t, dir, "untracked.txt", "new content\n")

	raw, _ := runGitAllowExitCodes(dir, nil, map[int]bool{0: true, 1: true},
		"diff", "--no-index", "--no-color", "--unified=3", "--", "/dev/null", path)
	if raw == "" {
		t.Skip("could not get raw diff")
	}

	out, err := ColorizeUntrackedDiff(dir, path, raw, false, 80, 3)
	if err != nil {
		t.Fatalf("ColorizeUntrackedDiff: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty colorized output")
	}
}

func TestDiffPathWithDelta_EmptyDiff(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	out, err := DiffPathWithDelta(dir, "README.md", false, false, 80, 3)
	if err != nil {
		t.Fatalf("DiffPathWithDelta: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output for no diff, got %q", out)
	}
}

func TestDiffPathWithDelta_StagedChange(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "delta.txt", "original\n")
	testutil.MustGitExported(t, dir, "add", "delta.txt")
	testutil.MustGitExported(t, dir, "commit", "-m", "init delta")
	testutil.WriteFile(t, dir, "delta.txt", "modified\n")
	testutil.MustGitExported(t, dir, "add", "delta.txt")

	out, err := DiffPathWithDelta(dir, "delta.txt", true, false, 80, 3)
	if err != nil {
		t.Fatalf("DiffPathWithDelta: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty diff output for staged change")
	}
}
