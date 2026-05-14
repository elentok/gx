package comments

import "testing"

func TestFormatMarkdownWithLocation(t *testing.T) {
	got := formatMarkdown("a/b.txt", "L10-12", []string{"-old", "+new"})
	want := "@a/b.txt L10-12\n\n```diff\n-old\n+new\n```\n"
	if got != want {
		t.Fatalf("formatMarkdown() = %q, want %q", got, want)
	}
}

func TestFormatMarkdownWithoutLocation(t *testing.T) {
	got := formatMarkdown("a/b.txt", "", []string{"diff --git a/b.txt b/b.txt"})
	want := "@a/b.txt\n\n```diff\ndiff --git a/b.txt b/b.txt\n```\n"
	if got != want {
		t.Fatalf("formatMarkdown() = %q, want %q", got, want)
	}
}
