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
