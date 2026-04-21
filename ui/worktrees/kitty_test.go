package worktrees

import "testing"

func TestKittySessionFile_AppendsSuffix(t *testing.T) {
	got := kittySessionFile("my-rp-ftr-a")
	want := "my-rp-ftr-a.kitty-session"
	if got != want {
		t.Fatalf("kittySessionFile() = %q, want %q", got, want)
	}
}
