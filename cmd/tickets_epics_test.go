package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestExecute_TicketsEpics_ListsBareSlugsSortedExcludingArchive(t *testing.T) {
	dir := testutil.TempRepo(t)
	scratchDir := filepath.Join(dir, ".scratch")
	for _, name := range []string{"zebra-epic", "alpha-epic", ".archive"} {
		if err := os.MkdirAll(filepath.Join(scratchDir, name), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	// A stray file directly under .scratch should not be treated as an epic.
	testutil.WriteFile(t, dir, ".scratch/notes.txt", "not an epic")

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return dir, nil },
	}

	if err := execute([]string{"tickets", "epics"}, d); err != nil {
		t.Fatalf("execute tickets epics: %v", err)
	}
	want := "alpha-epic\nzebra-epic\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestExecute_TicketsEpics_EmptyScratchExitsZeroWithNoOutput(t *testing.T) {
	dir := testutil.TempRepo(t)

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return dir, nil },
	}

	if err := execute([]string{"tickets", "epics"}, d); err != nil {
		t.Fatalf("execute tickets epics: %v", err)
	}
	if stdout.String() != "" {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestExecute_TicketsEpics_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: &stderr,
		getwd:  func() (string, error) { return dir, nil },
	}

	err := execute([]string{"tickets", "epics"}, d)
	if err == nil {
		t.Fatal("expected error when cwd is outside a git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a git repo") {
		t.Errorf("error = %q, want it to mention not being inside a git repo", err.Error())
	}
}
