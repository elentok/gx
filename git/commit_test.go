package git

import (
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestCommitDetailsForRef(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "first")
	testutil.WriteFile(t, repo, "a.txt", "two\n")
	testutil.CommitAll(t, repo, "second line\n\nbody line")
	testutil.MustGitExported(t, repo, "tag", "v1")

	got, err := CommitDetailsForRef(repo, "HEAD")
	if err != nil {
		t.Fatalf("CommitDetailsForRef: %v", err)
	}
	if got.Hash == "" || got.FullHash == "" {
		t.Fatalf("expected hashes, got %#v", got)
	}
	if got.Subject != "second line" {
		t.Fatalf("subject = %q", got.Subject)
	}
	if got.Body == "" || got.AuthorName == "" || got.AuthorShort == "" {
		t.Fatalf("expected populated details, got %#v", got)
	}
	foundTag := false
	for _, decoration := range got.Decorations {
		if decoration.Kind == RefDecorationTag && decoration.Name == "v1" {
			foundTag = true
		}
	}
	if !foundTag {
		t.Fatalf("expected v1 tag decoration, got %#v", got.Decorations)
	}
}

func TestCommitDetailsForRefNormalizesMixedNewlines(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "subject", "-m", "line 1\r\nline 2\nline 3\rline 4")

	got, err := CommitDetailsForRef(repo, "HEAD")
	if err != nil {
		t.Fatalf("CommitDetailsForRef: %v", err)
	}
	if got.Body != "subject\n\nline 1\nline 2\nline 3\nline 4" {
		t.Fatalf("body = %q", got.Body)
	}
}
