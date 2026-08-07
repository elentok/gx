package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestExecute_TicketsRoot_StandardRepo(t *testing.T) {
	dir := testutil.TempRepo(t)

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return dir, nil },
	}

	if err := execute([]string{"tickets", "root"}, d); err != nil {
		t.Fatalf("execute tickets root: %v", err)
	}
	want := filepath.Join(dir, ".scratch") + "\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestExecute_TicketsRoot_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: &stderr,
		getwd:  func() (string, error) { return dir, nil },
	}

	err := execute([]string{"tickets", "root"}, d)
	if err == nil {
		t.Fatal("expected error when cwd is outside a git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a git repo") {
		t.Errorf("error = %q, want it to mention not being inside a git repo", err.Error())
	}
}
