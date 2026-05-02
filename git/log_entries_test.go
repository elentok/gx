package git_test

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestLogEntries_StartsAtExactRef(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	testutil.WriteFile(t, repoDir, "one.txt", "one\n")
	testutil.CommitAll(t, repoDir, "one")
	testutil.WriteFile(t, repoDir, "two.txt", "two\n")
	testutil.CommitAll(t, repoDir, "two")

	ref, err := git.ResolveCommitish(repoDir, "HEAD~1")
	if err != nil {
		t.Fatalf("ResolveCommitish: %v", err)
	}

	entries, err := git.LogEntries(repoDir, "HEAD~1", 20)
	if err != nil {
		t.Fatalf("LogEntries: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}
	if entries[0].FullHash != ref {
		t.Fatalf("expected first entry to equal resolved ref %q, got %q", ref, entries[0].FullHash)
	}
	if entries[0].Subject != "one" {
		t.Fatalf("expected first entry subject one, got %q", entries[0].Subject)
	}
}

func TestLogEntries_ParsesTagDecoration(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	mustGit(t, repoDir, "tag", "v1.0.0")

	entries, err := git.LogEntries(repoDir, "HEAD", 1)
	if err != nil {
		t.Fatalf("LogEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	foundTag := false
	for _, decoration := range entries[0].Decorations {
		if decoration.Name == "v1.0.0" && decoration.Kind == git.RefDecorationTag {
			foundTag = true
		}
	}
	if !foundTag {
		t.Fatalf("expected tag decoration in %+v", entries[0].Decorations)
	}
}
