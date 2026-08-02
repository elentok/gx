package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestExecute_LogDispatchesToRunLog(t *testing.T) {
	var got LogOptions
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runLog: func(opts LogOptions) error {
			got = opts
			return nil
		},
	}

	if err := execute([]string{"log", "HEAD~2"}, d); err != nil {
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
	var got LogOptions
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runLog: func(opts LogOptions) error {
			got = opts
			return nil
		},
	}
	if err := execute([]string{"log", "abc123"}, d); err != nil {
		t.Fatalf("execute log abc123: %v", err)
	}
	if got.Ref != "abc123" {
		t.Fatalf("runLog ref = %q, want %q", got.Ref, "abc123")
	}
}

func TestExecute_LogFileFlag(t *testing.T) {
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
			repoDir := testutil.TempRepo(t)
			var got LogOptions
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				getwd:  func() (string, error) { return repoDir, nil },
				runLog: func(opts LogOptions) error {
					got = opts
					return nil
				},
			}
			if err := execute(tc.args, d); err != nil {
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
	repoDir := testutil.TempRepo(t)
	subDir := filepath.Join(repoDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	var got LogOptions
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return subDir, nil },
		runLog: func(opts LogOptions) error {
			got = opts
			return nil
		},
	}
	if err := execute([]string{"log", "-f", "file.txt"}, d); err != nil {
		t.Fatalf("execute log -f file.txt: %v", err)
	}
	if got.File != "sub/file.txt" {
		t.Fatalf("runLog file = %q, want %q", got.File, "sub/file.txt")
	}
}

func TestExecute_LogFileRejectsEscape(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
		runLog: func(LogOptions) error {
			t.Fatal("runLog should not be called when the path escapes the repo")
			return nil
		},
	}
	err := execute([]string{"log", "-f", "../outside.txt"}, d)
	if err == nil {
		t.Fatal("expected error for path escaping the repo root")
	}
}

func TestExecute_RunShow_DispatchesWithRef(t *testing.T) {
	var got string
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runShow: func(ref string) error {
			got = ref
			return nil
		},
	}
	if err := execute([]string{"show", "HEAD"}, d); err != nil {
		t.Fatalf("execute show HEAD: %v", err)
	}
	if got != "HEAD" {
		t.Fatalf("runShow ref = %q, want %q", got, "HEAD")
	}
}

func TestExecute_RunShow_NoRef(t *testing.T) {
	var got string
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runShow: func(ref string) error {
			got = ref
			return nil
		},
	}
	if err := execute([]string{"show"}, d); err != nil {
		t.Fatalf("execute show: %v", err)
	}
	if got != "" {
		t.Fatalf("runShow ref = %q, want empty", got)
	}
}
