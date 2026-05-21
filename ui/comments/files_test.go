package comments

import (
	"os"
	"strings"
	"testing"
)

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

func TestSanitizeFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello.go", "hello.go"},
		{"  hello  ", "hello"},
		{"hello world.go", "hello-world.go"},
		{"../../etc/passwd", "etc-passwd"},
		{"", ""},
		{"---", ""},
	}
	for _, c := range cases {
		got := sanitizeFilename(c.in)
		if got != c.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	filePath, err := Write("src/main.go", "L10", []string{"-old", "+new"})
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if !strings.Contains(string(data), "@src/main.go L10") {
		t.Errorf("file content missing expected header: %s", data)
	}
}

func TestWrite_EmptyPathUsesDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	filePath, err := Write("", "", []string{"line"})
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if !strings.HasSuffix(filePath, ".md") {
		t.Errorf("expected .md file, got %q", filePath)
	}
}
