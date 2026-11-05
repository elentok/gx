package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
)

// homeDeps is a Deps whose home directory is home.
func homeDeps(stdout, stderr *strings.Builder, home string) Deps {
	d := testDeps(stdout, stderr)
	d.UserHomeDir = func() (string, error) { return home, nil }
	return d
}

func TestRunClaudeSessionPath_PrintsMatchingPath(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := cmdtest.WriteSessionFixture(t, home, "-Users-david-dev-proj", "abc123", []string{`{"type":"user"}`})

	out := &strings.Builder{}
	d := homeDeps(out, nil, home)

	if err := runClaudeSessionPath(d, "abc123", ""); err != nil {
		t.Fatalf("runClaudeSessionPath: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != path {
		t.Fatalf("output = %q, want %q", got, path)
	}
}

func TestRunClaudeSessionPath_NoMatchIsError(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	out := &strings.Builder{}
	d := homeDeps(out, nil, home)

	err := runClaudeSessionPath(d, "missing-id", "")
	if err == nil {
		t.Fatal("expected an error for no matching transcript, got nil")
	}
}

func TestRunClaudeSessionPath_MultipleMatchesPrintsAllAndErrors(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	p1 := cmdtest.WriteSessionFixture(t, home, "-Users-david-dev-proj1", "dup-id", []string{`{"type":"user"}`})
	p2 := cmdtest.WriteSessionFixture(t, home, "-Users-david-dev-proj2", "dup-id", []string{`{"type":"user"}`})

	out := &strings.Builder{}
	errOut := &strings.Builder{}
	d := homeDeps(out, errOut, home)

	err := runClaudeSessionPath(d, "dup-id", "")
	if err == nil {
		t.Fatal("expected an error for multiple matching transcripts, got nil")
	}
	if out.String() != "" {
		t.Fatalf("expected stdout to be empty, got %q", out.String())
	}
	if !strings.Contains(errOut.String(), p1) || !strings.Contains(errOut.String(), p2) {
		t.Fatalf("expected both matches printed to stderr, got %q", errOut.String())
	}
}

func TestRunClaudeSessionPath_GrepFiltersLines(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cmdtest.WriteSessionFixture(t, home, "-Users-david-dev-proj", "grep-id", []string{
		`{"type":"user","text":"hello world"}`,
		`{"type":"assistant","text":"unrelated line"}`,
		`{"type":"user","text":"Hello again"}`,
	})

	out := &strings.Builder{}
	d := homeDeps(out, nil, home)

	if err := runClaudeSessionPath(d, "grep-id", "hello"); err != nil {
		t.Fatalf("runClaudeSessionPath: %v", err)
	}

	got := strings.TrimSpace(out.String())
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 matching lines, got %d: %q", len(lines), got)
	}
	if strings.Contains(got, "unrelated line") {
		t.Fatalf("expected non-matching line to be filtered out, got %q", got)
	}
}

func TestGrepFileLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.txt")
	content := strings.Join([]string{
		"Hello World",
		"goodbye world",
		"abc123",
		"xyz789",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	t.Run("matches case-insensitively", func(t *testing.T) {
		t.Parallel()
		lines, err := grepFileLines(path, "hello")
		if err != nil {
			t.Fatalf("grepFileLines: %v", err)
		}
		if len(lines) != 1 || lines[0] != "Hello World" {
			t.Fatalf("lines = %v, want [\"Hello World\"]", lines)
		}
	})

	t.Run("matches regex metacharacters", func(t *testing.T) {
		t.Parallel()
		lines, err := grepFileLines(path, "^[a-z]+[0-9]+$")
		if err != nil {
			t.Fatalf("grepFileLines: %v", err)
		}
		if len(lines) != 2 || lines[0] != "abc123" || lines[1] != "xyz789" {
			t.Fatalf("lines = %v, want [\"abc123\" \"xyz789\"]", lines)
		}
	})

	t.Run("invalid pattern returns an error", func(t *testing.T) {
		t.Parallel()
		_, err := grepFileLines(path, "[unclosed")
		if err == nil {
			t.Fatal("expected an error for an invalid regex pattern, got nil")
		}
	})
}
