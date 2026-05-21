package git

import (
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestRewordHead(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "file.txt", "content\n")
	testutil.MustGitExported(t, dir, "add", "file.txt")
	testutil.MustGitExported(t, dir, "commit", "-m", "original message")

	out, err := RewordHead(dir, "reworded message")
	if err != nil {
		t.Fatalf("RewordHead: %v\n%s", err, out)
	}

	// verify the commit message was changed
	msg, _, err2 := run(dir, []string{"log", "-1", "--format=%s"})
	if err2 != nil {
		t.Fatalf("git log: %v", err2)
	}
	if !strings.Contains(msg, "reworded message") {
		t.Errorf("commit message = %q, expected 'reworded message'", msg)
	}
}

func TestWriteTempRewordMessage(t *testing.T) {
	path, err := writeTempRewordMessage("test message")
	if err != nil {
		t.Fatalf("writeTempRewordMessage: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestWriteTempRewordScript(t *testing.T) {
	path, err := writeTempRewordScript("#!/bin/sh\necho hello\n")
	if err != nil {
		t.Fatalf("writeTempRewordScript: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}
