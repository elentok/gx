package git_test

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestParseVersion_Valid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tag                    string
		major, minor, patch    int
	}{
		{"v1.2.3", 1, 2, 3},
		{"v0.0.0", 0, 0, 0},
		{"v10.20.30", 10, 20, 30},
		{"1.2.3", 1, 2, 3}, // no v prefix
	}
	for _, tt := range tests {
		major, minor, patch, err := git.ParseVersion(tt.tag)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", tt.tag, err)
			continue
		}
		if major != tt.major || minor != tt.minor || patch != tt.patch {
			t.Errorf("ParseVersion(%q) = %d.%d.%d, want %d.%d.%d",
				tt.tag, major, minor, patch, tt.major, tt.minor, tt.patch)
		}
	}
}

func TestParseVersion_Invalid(t *testing.T) {
	t.Parallel()
	tests := []string{"v1.2", "v1", "vx.y.z", "", "v1.2.3.4"}
	for _, tag := range tests {
		_, _, _, err := git.ParseVersion(tag)
		if err == nil {
			t.Errorf("ParseVersion(%q) expected error, got nil", tag)
		}
	}
}

func TestLastTag_NoTags(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	tag := git.LastTag(dir)
	if tag != "v0.0.0" {
		t.Errorf("LastTag() = %q, want 'v0.0.0' for repo with no tags", tag)
	}
}

func TestLastTag_WithTag(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "README.md", "content")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "initial")
	testutil.MustGitExported(t, dir, "tag", "-a", "v1.2.3", "-m", "release")

	tag := git.LastTag(dir)
	if tag != "v1.2.3" {
		t.Errorf("LastTag() = %q, want 'v1.2.3'", tag)
	}
}
