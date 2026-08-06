package claudehistory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeJSONL writes a .jsonl file with the given records, each on its own line.
func writeJSONL(t *testing.T, path string, records []any) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			t.Fatal(err)
		}
	}
}

// setMtime sets the mtime of a file to the given time.
func setMtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestListProjectsRealCwd(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "-Users-alice-myproject")
	os.Mkdir(dir, 0o755)

	writeJSONL(t, filepath.Join(dir, "session1.jsonl"), []any{
		map[string]any{"cwd": "/Users/alice/myproject", "type": "system"},
		map[string]any{"type": "user", "message": "hello"},
	})

	projects, err := ListProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	p := projects[0]
	if p.Cwd != "/Users/alice/myproject" {
		t.Errorf("Cwd = %q, want /Users/alice/myproject", p.Cwd)
	}
	if p.Label != "myproject" {
		t.Errorf("Label = %q, want myproject", p.Label)
	}
}

func TestListProjectsDedashFallback(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "-Users-alice-project")
	os.Mkdir(dir, 0o755)

	// Transcript has no cwd field.
	writeJSONL(t, filepath.Join(dir, "session.jsonl"), []any{
		map[string]any{"type": "user", "message": "hi"},
	})

	projects, err := ListProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	p := projects[0]
	// Fallback: dedash "-Users-alice-project" → "/Users/alice/project"
	if !strings.HasPrefix(p.Cwd, "/") {
		t.Errorf("dedash fallback Cwd = %q, want something starting with /", p.Cwd)
	}
	if p.Label == "" {
		t.Error("expected non-empty Label from dedash fallback")
	}
}

func TestListProjectsWorktreeLabel(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "some-worktree-dir")
	os.Mkdir(dir, 0o755)

	cwd := "/Users/alice/myproject/.claude/worktrees/feature-branch"
	writeJSONL(t, filepath.Join(dir, "session.jsonl"), []any{
		map[string]any{"cwd": cwd},
	})

	projects, err := ListProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	p := projects[0]
	if !strings.Contains(p.Label, "»") {
		t.Errorf("worktree Label = %q, want parent » worktree format", p.Label)
	}
	if !strings.Contains(p.Label, "myproject") {
		t.Errorf("worktree Label = %q, should contain parent 'myproject'", p.Label)
	}
	if !strings.Contains(p.Label, "feature-branch") {
		t.Errorf("worktree Label = %q, should contain worktree name 'feature-branch'", p.Label)
	}
}

func TestListProjectsMtimeOrdering(t *testing.T) {
	root := t.TempDir()

	older := filepath.Join(root, "older-project")
	newer := filepath.Join(root, "newer-project")
	os.Mkdir(older, 0o755)
	os.Mkdir(newer, 0o755)

	now := time.Now()

	olderFile := filepath.Join(older, "s.jsonl")
	newerFile := filepath.Join(newer, "s.jsonl")

	writeJSONL(t, olderFile, []any{map[string]any{"cwd": "/old"}})
	writeJSONL(t, newerFile, []any{map[string]any{"cwd": "/new"}})

	setMtime(t, olderFile, now.Add(-2*time.Hour))
	setMtime(t, newerFile, now.Add(-1*time.Hour))

	projects, err := ListProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].Cwd != "/new" {
		t.Errorf("expected newest first: got %q, want /new", projects[0].Cwd)
	}
	if projects[1].Cwd != "/old" {
		t.Errorf("expected oldest last: got %q, want /old", projects[1].Cwd)
	}
}

func TestListProjectsCollapseTilde(t *testing.T) {
	root := t.TempDir()
	home, _ := os.UserHomeDir()

	dir := filepath.Join(root, "my-project")
	os.Mkdir(dir, 0o755)
	cwd := filepath.Join(home, "work", "myproject")
	writeJSONL(t, filepath.Join(dir, "s.jsonl"), []any{map[string]any{"cwd": cwd}})

	projects, err := ListProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	p := projects[0]
	if !strings.HasPrefix(p.Subtitle, "~") {
		t.Errorf("Subtitle = %q, want ~ prefix for home dir", p.Subtitle)
	}
}

func TestListProjectsSkipsEmptyDirs(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "empty-dir"), 0o755)

	projects, err := ListProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects (no .jsonl files), got %d", len(projects))
	}
}

func TestListProjectsNonExistentRoot(t *testing.T) {
	projects, err := ListProjects("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects for nonexistent root, got %d", len(projects))
	}
}

func TestDedashDir(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"-Users-alice-project", "/Users/alice/project"},
		{"-home-bob-work", "/home/bob/work"},
		{"noLeadingDash", "noLeadingDash"},
	}
	for _, c := range cases {
		got := dedashDir(c.in)
		if got != c.want {
			t.Errorf("dedashDir(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCollapseTilde(t *testing.T) {
	home := "/Users/alice"
	cases := []struct {
		path string
		want string
	}{
		{"/Users/alice/project", "~/project"},
		{"/Users/alice", "~"},
		{"/other/path", "/other/path"},
	}
	for _, c := range cases {
		got := collapseTilde(c.path, home)
		if got != c.want {
			t.Errorf("collapseTilde(%q, %q) = %q, want %q", c.path, home, got, c.want)
		}
	}
}

func TestBuildLabelWorktree(t *testing.T) {
	cases := []struct {
		cwd      string
		contains []string
	}{
		{
			cwd:      "/Users/alice/proj/.claude/worktrees/feat",
			contains: []string{"proj", "feat", "»"},
		},
		{
			cwd:      "/Users/alice/proj/worktrees/my-wt",
			contains: []string{"proj", "my-wt", "»"},
		},
		{
			cwd:      "/Users/alice/regular-project",
			contains: []string{"regular-project"},
		},
	}
	for _, c := range cases {
		got := buildLabel(c.cwd)
		for _, want := range c.contains {
			if !strings.Contains(got, want) {
				t.Errorf("buildLabel(%q) = %q, want it to contain %q", c.cwd, got, want)
			}
		}
	}
}
