package worktrees

import "testing"

func TestKittySessionFile_AppendsSuffix(t *testing.T) {
	got := kittySessionFile("my-rp-ftr-a")
	want := "my-rp-ftr-a.kitty-session"
	if got != want {
		t.Fatalf("kittySessionFile() = %q, want %q", got, want)
	}
}

func TestSessionNameFor_CompressesRepoAndWorktree(t *testing.T) {
	got := sessionNameFor("my-repo", "feature-a")
	want := "my-rpo-ftre-a"
	if got != want {
		t.Fatalf("sessionNameFor() = %q, want %q", got, want)
	}
}

func TestSessionNameFor_DotBareNameIsNotUsedAsRepoName(t *testing.T) {
	got := sessionNameFor("my-repo", "main")
	if got == "bre-mn" || got == "bre-main" || got == ".bre-mn" || got == ".bre-main" {
		t.Fatalf("session name should use outer repo dir, got %q", got)
	}
}
