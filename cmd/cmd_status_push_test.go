package cmd

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestExecute_RunsStatus(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"status", "s"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			called := 0
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				runStatus: func(string) error {
					called++
					return nil
				},
			}

			if err := execute([]string{name}, d); err != nil {
				t.Fatalf("execute %s: %v", name, err)
			}
			if called != 1 {
				t.Fatalf("runStatus called %d times, want 1", called)
			}
		})
	}
}

func TestExecute_RunsStatusWithPath(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "relative", args: []string{"status", "README.md"}, want: "README.md"},
		{name: "alias", args: []string{"s", "/tmp/file.txt"}, want: "/tmp/file.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got string
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				runStatus: func(path string) error {
					got = path
					return nil
				},
			}

			if err := execute(tc.args, d); err != nil {
				t.Fatalf("execute %v: %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("runStatus path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveStatusTargetPath(t *testing.T) {
	t.Parallel()
	root := "/repo"
	cwd := "/repo/sub"

	got, err := resolveStatusTargetPath(root, cwd, "file.txt")
	if err != nil {
		t.Fatalf("resolveStatusTargetPath relative: %v", err)
	}
	if got != "sub/file.txt" {
		t.Fatalf("relative target = %q, want %q", got, "sub/file.txt")
	}

	abs := filepath.Join(root, "deep", "file.txt")
	got, err = resolveStatusTargetPath(root, cwd, abs)
	if err != nil {
		t.Fatalf("resolveStatusTargetPath absolute: %v", err)
	}
	if got != "deep/file.txt" {
		t.Fatalf("absolute target = %q, want %q", got, "deep/file.txt")
	}
}

func TestResolveStatusTargetPathRejectsOutsideWorktree(t *testing.T) {
	t.Parallel()
	_, err := resolveStatusTargetPath("/repo", "/repo", "../other.txt")
	if err == nil {
		t.Fatal("expected error for path outside worktree")
	}
}

func TestResolveStatusTargetPath_ExactRoot(t *testing.T) {
	t.Parallel()
	_, err := resolveStatusTargetPath("/repo", "/repo", "/repo")
	if err == nil {
		t.Fatal("expected error for exact root target")
	}
}
