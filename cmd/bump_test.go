package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"

	tea "charm.land/bubbletea/v2"
)

// tagRepo creates a git tag in the given repo directory.
func tagRepo(t *testing.T, dir, tag string) {
	t.Helper()
	testutil.MustGitExported(t, dir, "tag", "-a", tag, "-m", "Release "+tag)
}

// --- parseVersion ---

func TestParseVersion_Valid(t *testing.T) {
	tests := []struct {
		tag                 string
		major, minor, patch int
	}{
		{"v1.2.3", 1, 2, 3},
		{"v0.0.0", 0, 0, 0},
		{"v10.20.30", 10, 20, 30},
		{"1.2.3", 1, 2, 3}, // no "v" prefix
	}
	for _, tt := range tests {
		major, minor, patch, err := parseVersion(tt.tag)
		if err != nil {
			t.Errorf("parseVersion(%q) error: %v", tt.tag, err)
			continue
		}
		if major != tt.major || minor != tt.minor || patch != tt.patch {
			t.Errorf("parseVersion(%q) = %d.%d.%d, want %d.%d.%d",
				tt.tag, major, minor, patch, tt.major, tt.minor, tt.patch)
		}
	}
}

func TestParseVersion_Invalid(t *testing.T) {
	for _, tag := range []string{"v1.2", "v1", "vx.y.z", "v1.2.x", ""} {
		_, _, _, err := parseVersion(tag)
		if err == nil {
			t.Errorf("parseVersion(%q): expected error, got nil", tag)
		}
	}
}

// --- pickBump ---

func TestPickBump_EnterSelectsPatch(t *testing.T) {
	got, err := pickBump("v1.2.3", 1, 2, 3, strings.NewReader("\r"), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("pickBump: %v", err)
	}
	if got != "patch" {
		t.Errorf("got %q, want %q", got, "patch")
	}
}

func TestPickBump_DownThenEnterSelectsMinor(t *testing.T) {
	got, err := pickBump("v1.2.3", 1, 2, 3, strings.NewReader("j\r"), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("pickBump: %v", err)
	}
	if got != "minor" {
		t.Errorf("got %q, want %q", got, "minor")
	}
}

func TestPickBump_CursorClampsAtBottom(t *testing.T) {
	// Verify that pressing j past the last option does not wrap and still
	// selects the last option (major) on enter. The picker has 3 options
	// (patch=0, minor=1, major=2), so pressing j three times (one more than
	// needed) should clamp at index 2.
	m := bumpPickerModel{
		lastTag: "v1.2.3",
		options: []bumpOption{
			{"patch", "v1.2.4"},
			{"minor", "v1.3.0"},
			{"major", "v2.0.0"},
		},
	}
	// Simulate three j presses then enter.
	for range 3 {
		next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = next.(bumpPickerModel)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(bumpPickerModel)
	if m.chosen != "major" {
		t.Errorf("chosen = %q, want %q", m.chosen, "major")
	}
}

func TestPickBump_QCancels(t *testing.T) {
	got, err := pickBump("v1.2.3", 1, 2, 3, strings.NewReader("q"), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("pickBump: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string (cancelled)", got)
	}
}

func TestPickBump_EscCancels(t *testing.T) {
	got, err := pickBump("v1.2.3", 1, 2, 3, strings.NewReader("\x1b"), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("pickBump: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string (cancelled)", got)
	}
}

// --- runBump with explicit args ---

func TestRunBump_PatchExplicit(t *testing.T) {
	dir := testutil.TempRepo(t)
	tagRepo(t, dir, "v1.2.3")

	var stdout bytes.Buffer
	d := deps{
		stdin:        strings.NewReader(""),
		stdout:       &stdout,
		stderr:       bytes.NewBuffer(nil),
		getwd:        func() (string, error) { return dir, nil },
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	if err := runBump([]string{"patch"}, d); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "v1.2.4") {
		t.Errorf("expected v1.2.4 in output, got: %q", out)
	}
}

func TestRunBump_MinorExplicit(t *testing.T) {
	dir := testutil.TempRepo(t)
	tagRepo(t, dir, "v1.2.3")

	var stdout bytes.Buffer
	d := deps{
		stdin:        strings.NewReader(""),
		stdout:       &stdout,
		stderr:       bytes.NewBuffer(nil),
		getwd:        func() (string, error) { return dir, nil },
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	if err := runBump([]string{"minor"}, d); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "v1.3.0") {
		t.Errorf("expected v1.3.0 in output, got: %q", out)
	}
}

func TestRunBump_MajorExplicit(t *testing.T) {
	dir := testutil.TempRepo(t)
	tagRepo(t, dir, "v1.2.3")

	var stdout bytes.Buffer
	d := deps{
		stdin:        strings.NewReader(""),
		stdout:       &stdout,
		stderr:       bytes.NewBuffer(nil),
		getwd:        func() (string, error) { return dir, nil },
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	if err := runBump([]string{"major"}, d); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "v2.0.0") {
		t.Errorf("expected v2.0.0 in output, got: %q", out)
	}
}

func TestRunBump_NoExistingTag_DefaultsToV0(t *testing.T) {
	dir := testutil.TempRepo(t)

	var stdout bytes.Buffer
	d := deps{
		stdin:        strings.NewReader(""),
		stdout:       &stdout,
		stderr:       bytes.NewBuffer(nil),
		getwd:        func() (string, error) { return dir, nil },
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	if err := runBump([]string{"patch"}, d); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "v0.0.1") {
		t.Errorf("expected v0.0.1 in output, got: %q", out)
	}
}

func TestRunBump_CreatesAnnotatedTag(t *testing.T) {
	dir := testutil.TempRepo(t)
	tagRepo(t, dir, "v1.0.0")

	d := deps{
		stdin:        strings.NewReader(""),
		stdout:       bytes.NewBuffer(nil),
		stderr:       bytes.NewBuffer(nil),
		getwd:        func() (string, error) { return dir, nil },
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	if err := runBump([]string{"patch"}, d); err != nil {
		t.Fatalf("runBump: %v", err)
	}

	// Verify the tag was created.
	out, err := gitOutput(dir, "tag", "-l", "v1.0.1")
	if err != nil {
		t.Fatalf("git tag -l: %v", err)
	}
	if strings.TrimSpace(out) != "v1.0.1" {
		t.Errorf("tag v1.0.1 not found; git tag -l output: %q", out)
	}

	// Verify it is annotated (has a tag object).
	objType, err := gitOutput(dir, "cat-file", "-t", "v1.0.1")
	if err != nil {
		t.Fatalf("git cat-file -t: %v", err)
	}
	if strings.TrimSpace(objType) != "tag" {
		t.Errorf("tag type = %q, want %q", objType, "tag")
	}
}

func TestRunBump_SkipsPushWhenDeclined(t *testing.T) {
	dir := testutil.TempRepo(t)
	tagRepo(t, dir, "v1.0.0")

	var stdout bytes.Buffer
	d := deps{
		stdin:        strings.NewReader(""),
		stdout:       &stdout,
		stderr:       bytes.NewBuffer(nil),
		getwd:        func() (string, error) { return dir, nil },
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	if err := runBump([]string{"patch"}, d); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	if strings.Contains(stdout.String(), "Pushed") {
		t.Errorf("expected push to be skipped, got: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Skipped") {
		t.Errorf("expected 'Skipped' in output, got: %q", stdout.String())
	}
}

func TestRunBump_InteractivePicker(t *testing.T) {
	dir := testutil.TempRepo(t)
	tagRepo(t, dir, "v2.0.0")

	var stdout bytes.Buffer
	d := deps{
		// "j\r" = move down once, then enter → picks "minor"
		stdin:        strings.NewReader("j\r"),
		stdout:       &stdout,
		stderr:       bytes.NewBuffer(nil),
		getwd:        func() (string, error) { return dir, nil },
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	if err := runBump(nil, d); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	if !strings.Contains(stdout.String(), "v2.1.0") {
		t.Errorf("expected v2.1.0 in output, got: %q", stdout.String())
	}
}

func TestRunBump_InteractivePicker_Cancel(t *testing.T) {
	dir := testutil.TempRepo(t)
	tagRepo(t, dir, "v1.0.0")

	var stdout bytes.Buffer
	d := deps{
		stdin:        strings.NewReader("q"),
		stdout:       &stdout,
		stderr:       bytes.NewBuffer(nil),
		getwd:        func() (string, error) { return dir, nil },
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	if err := runBump(nil, d); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	// Cancelled — no tag should be created, nothing pushed.
	if strings.Contains(stdout.String(), "Bumping") {
		t.Errorf("expected no bump output on cancel, got: %q", stdout.String())
	}
}
