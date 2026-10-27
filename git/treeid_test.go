package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTreeID(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "user.email", "t@example.com")
	gitIn(t, dir, "config", "user.name", "t")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	treeID := func() string {
		t.Helper()
		id, err := TreeID(dir)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	// A repo with no commits has a tree too.
	write(".gitignore", "ignored.txt\n")
	write("a.txt", "a\n")
	before := treeID()

	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "first")
	if got := treeID(); got != before {
		t.Errorf("tree after commit = %s, want %s", got, before)
	}

	write("ignored.txt", "x\n")
	if got := treeID(); got != before {
		t.Errorf("an ignored file changed the tree: %s", got)
	}

	write("new.txt", "n\n")
	withUntracked := treeID()
	if withUntracked == before {
		t.Error("an untracked file did not change the tree")
	}

	gitIn(t, dir, "add", "new.txt")
	if got := treeID(); got != withUntracked {
		t.Errorf("staging changed the tree: %s", got)
	}
	if out, _, _ := run(dir, []string{"diff", "--cached", "--name-only"}); out != "new.txt" {
		t.Errorf("real index = %q, want only new.txt staged", out)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, stderr, err := run(dir, args); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, stderr)
	}
}
