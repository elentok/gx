package git

import (
	"os"
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
	msg := "test message\nsecond line"
	path, err := writeTempRewordMessage(msg)
	if err != nil {
		t.Fatalf("writeTempRewordMessage: %v", err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != msg {
		t.Errorf("content = %q, want %q", string(data), msg)
	}
}

func TestWriteTempRewordScript(t *testing.T) {
	content := "#!/bin/sh\necho hello\n"
	path, err := writeTempRewordScript(content)
	if err != nil {
		t.Fatalf("writeTempRewordScript: %v", err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != content {
		t.Errorf("content = %q, want %q", string(data), content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode()&0100 == 0 {
		t.Error("expected script to be executable")
	}
}

func TestRewordCommit(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "target.txt", "content\n")
	testutil.MustGitExported(t, dir, "add", "target.txt")
	testutil.MustGitExported(t, dir, "commit", "-m", "original message")
	testutil.WriteFile(t, dir, "tip.txt", "tip\n")
	testutil.CommitAll(t, dir, "tip commit")

	log, _, err := run(dir, []string{"log", "--format=%H", "-n", "2"})
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(log), "\n")
	targetHash := lines[1]

	out, err := RewordCommit(dir, targetHash, "reworded message")
	if err != nil {
		t.Fatalf("RewordCommit: %v\n%s", err, out)
	}

	fullLog, _, err := run(dir, []string{"log", "--format=%s"})
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(fullLog, "reworded message") {
		t.Errorf("commit message not changed; log = %q", fullLog)
	}
}
