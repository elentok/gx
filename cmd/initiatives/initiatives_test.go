package initiatives

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestMain(m *testing.M) { cmdtest.Main(m) }

// setup is an isolated store holding project "mine" and its repo. The env has
// no server, so a server call would fail the command.
func setup(t *testing.T) (repo, project string) {
	t.Helper()
	store := cmdtest.IsolateTicketStore(t)
	repo = testutil.TempRepo(t)
	return repo, cmdtest.AddProject(t, store, "mine", repo)
}

func run(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	env := cmdbase.Env{
		Stdout: &out,
		Getwd:  func() (string, error) { return cwd, nil },
		Env:    cmdtest.EnvFor(t),
	}
	err := cmdtest.Run(t, env, append([]string{"initiatives"}, args...), New(Deps{Env: env}))
	return out.String(), err
}

func epicDirs(t *testing.T, project string) []string {
	t.Helper()
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func TestNew_CreatesAnOpenInitiativeHoldingOnlyTheIdea(t *testing.T) {
	repo, project := setup(t)

	out, err := run(t, repo, "new", "mine:demo-init", "make things faster")
	if err != nil || out != "mine:demo-init\n" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	epic := tickets.LoadEpicTicketMD(filepath.Join(project, "demo-init"))
	if !epic.IsInitiative || epic.Status != "open" {
		t.Errorf("IsInitiative = %v status = %q", epic.IsInitiative, epic.Status)
	}
	if want := "## Initial prompt\n\nmake things faster"; strings.TrimSpace(epic.Body) != want {
		t.Errorf("body = %q, want %q", epic.Body, want)
	}
	raw, err := os.ReadFile(filepath.Join(project, "demo-init", "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "design_step") {
		t.Errorf("ticket.md has a design_step:\n%s", raw)
	}
	if got := epicDirs(t, project); len(got) != 1 {
		t.Errorf("epic dirs = %v, want only demo-init", got)
	}
}

func TestNew_MakesNoBranchAndNoWorktree(t *testing.T) {
	repo, _ := setup(t)
	if _, err := run(t, repo, "new", "mine:demo-init", "idea"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "refs", "heads", "demo-init")); err == nil {
		t.Error("a branch demo-init was created")
	}
	entries, err := os.ReadDir(filepath.Join(repo, ".git", "worktrees"))
	if err == nil && len(entries) > 0 {
		t.Errorf("worktrees created: %v", entries)
	}
}

func TestNew_RefusesAClashAndCreatesNothing(t *testing.T) {
	repo, project := setup(t)
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "taken"))
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "other-design"))
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, ".archive", "old"))
	testutil.MustGitExported(t, repo, "branch", "branched")

	for _, tc := range []struct{ slug, wantErr string }{
		{"taken", `epic "taken" already exists`},
		{"other", `epic "other-design" already exists`},
		{"old", "in the archive"},
		{"branched", `branch "branched" exists`},
	} {
		before := epicDirs(t, project)
		out, err := run(t, repo, "new", "mine:"+tc.slug, "idea")
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: err = %v, want containing %q", tc.slug, err, tc.wantErr)
		}
		if out != "" || strings.Join(epicDirs(t, project), ",") != strings.Join(before, ",") {
			t.Errorf("%s: out = %q, epics changed to %v", tc.slug, out, epicDirs(t, project))
		}
	}
}

func TestNew_RefusesAnUnknownProjectAndAnEmptyIdea(t *testing.T) {
	repo, project := setup(t)
	_, err := run(t, repo, "new", "nope:demo", "idea")
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeUnknownProject {
		t.Errorf("unknown project: err = %v", err)
	}
	if _, err := run(t, repo, "new", "mine:demo", "  "); err == nil {
		t.Error("empty idea: want an error")
	}
	if got := epicDirs(t, project); len(got) != 0 {
		t.Errorf("epics created: %v", got)
	}
}

func TestNew_BareSlugUsesTheCwdProject(t *testing.T) {
	repo, project := setup(t)
	out, err := run(t, repo, "new", "bare-init", "idea")
	if err != nil || out != "mine:bare-init\n" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(project, "bare-init", "ticket.md")); err != nil {
		t.Error(err)
	}
}
