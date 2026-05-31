package worktrees

import "testing"

func TestKittySessionFile_AppendsSuffix(t *testing.T) {
	got := kittySessionFile("my-repo-feature-a")
	want := "my-repo-feature-a.kitty-session"
	if got != want {
		t.Fatalf("kittySessionFile() = %q, want %q", got, want)
	}
}

func TestSessionNameFor_UsesFullRepoAndWorktree(t *testing.T) {
	got := sessionNameFor("my-repo", "feature-a", nil)
	want := "my-repo-feature-a"
	if got != want {
		t.Fatalf("sessionNameFor() = %q, want %q", got, want)
	}
}

func TestSessionNameFor_AppliesAliasesWithoutShortening(t *testing.T) {
	got := sessionNameFor("my-project-frontend", "feature-super-long", map[string]string{
		"my-project-frontend": "proj",
		"feature-super-long":  "feat",
	})
	want := "proj-feat"
	if got != want {
		t.Fatalf("sessionNameFor() = %q, want %q", got, want)
	}
}

func TestSessionNameFor_DotBareNameIsNotUsedAsRepoName(t *testing.T) {
	got := sessionNameFor("my-repo", "main", nil)
	if got == "bre-mn" || got == "bre-main" || got == ".bre-mn" || got == ".bre-main" {
		t.Fatalf("session name should use outer repo dir, got %q", got)
	}
}
