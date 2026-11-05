package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/testutil"
)

func TestExecute_LogDispatchesToRunLog(t *testing.T) {
	t.Parallel()
	var got LogOptions
	d := Deps{
		Env: cmdbase.Env{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil)},
		RunLog: func(opts LogOptions) error {
			got = opts
			return nil
		},
	}

	if err := cmdtest.Run(t, d.Env, []string{"log", "HEAD~2"}, NewLog(d)); err != nil {
		t.Fatalf("execute log: %v", err)
	}
	if got.Ref != "HEAD~2" {
		t.Fatalf("runLog called with ref %q, want %q", got.Ref, "HEAD~2")
	}
	if got.File != "" {
		t.Fatalf("runLog called with file %q, want empty", got.File)
	}
}

func TestExecute_RunLog_DispatchesWithRef(t *testing.T) {
	t.Parallel()
	var got LogOptions
	d := Deps{
		Env: cmdbase.Env{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil)},
		RunLog: func(opts LogOptions) error {
			got = opts
			return nil
		},
	}
	if err := cmdtest.Run(t, d.Env, []string{"log", "abc123"}, NewLog(d)); err != nil {
		t.Fatalf("execute log abc123: %v", err)
	}
	if got.Ref != "abc123" {
		t.Fatalf("runLog ref = %q, want %q", got.Ref, "abc123")
	}
}

func TestExecute_LogFileFlag(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		wantRef string
	}{
		{name: "short", args: []string{"log", "-f", "foo.txt"}, wantRef: ""},
		{name: "long", args: []string{"log", "--file", "foo.txt"}, wantRef: ""},
		{name: "long-equals", args: []string{"log", "--file=foo.txt"}, wantRef: ""},
		{name: "flag-before-ref", args: []string{"log", "-f", "foo.txt", "abc123"}, wantRef: "abc123"},
		{name: "ref-before-flag", args: []string{"log", "abc123", "-f", "foo.txt"}, wantRef: "abc123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repoDir := testutil.TempRepo(t)
			var got LogOptions
			d := Deps{
				Env: cmdbase.Env{
					Stdout: bytes.NewBuffer(nil),
					Stderr: bytes.NewBuffer(nil),
					Getwd:  func() (string, error) { return repoDir, nil },
				},
				RunLog: func(opts LogOptions) error {
					got = opts
					return nil
				},
			}
			if err := cmdtest.Run(t, d.Env, tc.args, NewLog(d)); err != nil {
				t.Fatalf("execute %v: %v", tc.args, err)
			}
			if got.Ref != tc.wantRef {
				t.Fatalf("runLog ref = %q, want %q", got.Ref, tc.wantRef)
			}
			if got.File != "foo.txt" {
				t.Fatalf("runLog file = %q, want %q", got.File, "foo.txt")
			}
		})
	}
}

func TestExecute_LogFileResolvesFromSubdir(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	subDir := filepath.Join(repoDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	var got LogOptions
	d := Deps{
		Env: cmdbase.Env{
			Stdout: bytes.NewBuffer(nil),
			Stderr: bytes.NewBuffer(nil),
			Getwd:  func() (string, error) { return subDir, nil },
		},
		RunLog: func(opts LogOptions) error {
			got = opts
			return nil
		},
	}
	if err := cmdtest.Run(t, d.Env, []string{"log", "-f", "file.txt"}, NewLog(d)); err != nil {
		t.Fatalf("execute log -f file.txt: %v", err)
	}
	if got.File != "sub/file.txt" {
		t.Fatalf("runLog file = %q, want %q", got.File, "sub/file.txt")
	}
}

func TestExecute_LogFileRejectsEscape(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	d := Deps{
		Env: cmdbase.Env{
			Stdout: bytes.NewBuffer(nil),
			Stderr: bytes.NewBuffer(nil),
			Getwd:  func() (string, error) { return repoDir, nil },
		},
		RunLog: func(LogOptions) error {
			t.Fatal("runLog should not be called when the path escapes the repo")
			return nil
		},
	}
	err := cmdtest.Run(t, d.Env, []string{"log", "-f", "../outside.txt"}, NewLog(d))
	if err == nil {
		t.Fatal("expected error for path escaping the repo root")
	}
}

func TestExecute_RunShow_DispatchesWithRef(t *testing.T) {
	t.Parallel()
	var got string
	d := Deps{
		Env: cmdbase.Env{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil)},
		RunShow: func(ref string) error {
			got = ref
			return nil
		},
	}
	if err := cmdtest.Run(t, d.Env, []string{"show", "HEAD"}, NewShow(d)); err != nil {
		t.Fatalf("execute show HEAD: %v", err)
	}
	if got != "HEAD" {
		t.Fatalf("runShow ref = %q, want %q", got, "HEAD")
	}
}

func TestExecute_RunShow_NoRef(t *testing.T) {
	t.Parallel()
	var got string
	d := Deps{
		Env: cmdbase.Env{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil)},
		RunShow: func(ref string) error {
			got = ref
			return nil
		},
	}
	if err := cmdtest.Run(t, d.Env, []string{"show"}, NewShow(d)); err != nil {
		t.Fatalf("execute show: %v", err)
	}
	if got != "" {
		t.Fatalf("runShow ref = %q, want empty", got)
	}
}
