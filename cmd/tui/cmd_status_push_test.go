package tui

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
)

func TestExecute_RunsStatus(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"status", "s"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			called := 0
			d := Deps{
				Env: cmdbase.Env{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil)},
				RunStatus: func(string) error {
					called++
					return nil
				},
			}

			if err := cmdtest.Run(t, d.Env, []string{name}, NewStatus(d)); err != nil {
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
			d := Deps{
				Env: cmdbase.Env{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil)},
				RunStatus: func(path string) error {
					got = path
					return nil
				},
			}

			if err := cmdtest.Run(t, d.Env, tc.args, NewStatus(d)); err != nil {
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
