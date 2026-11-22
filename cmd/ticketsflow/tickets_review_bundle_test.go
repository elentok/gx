package ticketsflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
)

// reviewBundleRepo builds a repo on main with one commit, then a feature
// branch whose changes are files (path -> content). It returns the repo and
// the isolated store with project "proj" and epic "epic" registered to it.
func reviewBundleRepo(t *testing.T, files map[string]string) (repo, store string) {
	t.Helper()
	store = isolateTicketStore(t)
	repo = t.TempDir()
	env := envFor(t)
	gitRun := func(args ...string) {
		t.Helper()
		if _, stderr, err := git.Run(repo, env, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, stderr)
		}
	}
	gitRun("init", "-b", "main")
	gitRun("config", "user.email", "t@example.com")
	gitRun("config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun("add", ".")
	gitRun("commit", "-m", "base")
	gitRun("checkout", "-b", "ralph-loop/epic")
	for name, content := range files {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(files) > 0 {
		gitRun("add", ".")
		gitRun("commit", "-m", "epic/01: change things")
	}
	scratch := addProject(t, store, "proj", repo)
	ticket := func(file, id, status, typ, body string) {
		fm := "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: " + typ + "\n---\n" + body
		writeEpicTicket(t, scratch, "epic", file, fm)
	}
	ticket("01-impl.md", "01", "done", "implement", "\n# Impl\n\n## Spec deviations\n\n- narrowed the thing\n")
	ticket("02-review.md", "02", "claimed", "code-review", "\n# Review\n")
	ticket("03-late.md", "03", "open", "implement", "\n# Late fix\n")
	return repo, store
}

func TestExecute_TicketsReviewBundle_SplitsByAreaAndCapsSize(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("line of production code\n", 9000) // ~216 KB of diff
	repo, _ := reviewBundleRepo(t, map[string]string{
		"pkg/a.go":              big,
		"pkg/a_test.go":         "package pkg\n",
		"webui/src/App.tsx":     "export {}\n",
		"docs/notes.md":         "# notes\n",
		"pkg/testdata/f.json":   "{}\n",
		"pnpm-lock.yaml":        "lock\n",
		"docs/specs/epic.md":    "# spec\n",
		"skills/x/SKILL.md":     "# skill\n",
		"webui/e2e/flow.e2e.ts": "export {}\n",
	})
	out := filepath.Join(t.TempDir(), "bundle")
	text, err := runIn(t, repo, "tickets", "review-bundle", "proj:epic", "--out", out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"merge-base:", "epic/01: change things", "docs/specs/epic.md", "gx-code-reviewer", "index.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("stdout missing %q:\n%s", want, text)
		}
	}

	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	var goParts int
	got := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(out, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 100*1024 {
			t.Errorf("%s is %d bytes, want at most 100 KB", e.Name(), len(data))
		}
		got[e.Name()] = string(data)
		if strings.HasPrefix(e.Name(), "diff-go") {
			goParts++
		}
	}
	if goParts < 2 {
		t.Errorf("go diff parts = %d, want the big file split over at least 2", goParts)
	}
	all := ""
	for name, content := range got {
		if strings.HasSuffix(name, ".diff") {
			all += "\n" + name + "\n" + content
		}
	}
	for _, want := range []string{"diff-tests", "diff-web", "diff-docs", "a_test.go", "App.tsx", "notes.md"} {
		if !strings.Contains(all, want) {
			t.Errorf("diff files missing %q", want)
		}
	}
	// Fixtures and lockfiles stay out of every diff; docs stay out of code diffs.
	for _, bad := range []string{"testdata/f.json", "pnpm-lock.yaml"} {
		if strings.Contains(all, bad) {
			t.Errorf("diffs contain excluded %q", bad)
		}
	}
	for name, content := range got {
		if strings.HasPrefix(name, "diff-go") && strings.Contains(content, "notes.md") {
			t.Errorf("%s holds a docs file", name)
		}
	}
	if idx := got["index.md"]; !strings.Contains(idx, "pnpm-lock.yaml") || !strings.Contains(idx, "excluded") {
		t.Errorf("index.md does not list excluded files:\n%s", idx)
	}
}

func TestExecute_TicketsReviewBundle_ListsDeviationsAndPostHandoffTickets(t *testing.T) {
	t.Parallel()
	repo, _ := reviewBundleRepo(t, map[string]string{"pkg/a.go": "package pkg\n"})
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := runIn(t, repo, "tickets", "review-bundle", "proj:epic", "--out", out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "deviations.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"narrowed the thing", "proj:epic/01", "proj:epic/03", "Late fix"} {
		if !strings.Contains(text, want) {
			t.Errorf("deviations.md missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "proj:epic/02") {
		t.Errorf("the code-review ticket itself is listed:\n%s", text)
	}
}

func TestExecute_TicketsReviewBundle_EmptyDiffFailsEarly(t *testing.T) {
	t.Parallel()
	repo, _ := reviewBundleRepo(t, nil)
	out := filepath.Join(t.TempDir(), "bundle")
	_, err := runIn(t, repo, "tickets", "review-bundle", "proj:epic", "--out", out)
	if err == nil || !strings.Contains(err.Error(), "empty diff") {
		t.Fatalf("err = %v, want an empty diff error", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Errorf("bundle dir was created for an empty diff")
	}
}
