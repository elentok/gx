package worktrees

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	keymgr "github.com/elentok/gx/ui/keys"
)

// trunkFixture is a repo registered as project "p" with a "develop" worktree
// one commit ahead of main and a "feature" worktree one commit ahead of main.
type trunkFixture struct {
	repo       git.Repo
	projectDir string
	store      string
}

func newTrunkFixture(t *testing.T, trunk string) trunkFixture {
	t.Helper()
	repoDir := testutil.TempRepo(t)
	wtDir := t.TempDir()
	for _, name := range []string{"develop", "feature"} {
		path := filepath.Join(wtDir, name)
		testutil.MustGitExported(t, repoDir, "worktree", "add", "-b", name, path)
		testutil.WriteFile(t, path, name+".txt", name)
		testutil.CommitAll(t, path, name)
	}

	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	projectDir, store := testutil.RegisterProject(t, repo.Root, trunk)
	return trunkFixture{repo: *repo, projectDir: projectDir, store: store}
}

func (f trunkFixture) model() Model {
	return NewWithSettings(f.repo, "", ui.Settings{TicketStore: f.store}, keymgr.Manager{})
}

func (f trunkFixture) setTrunk(t *testing.T, trunk string) {
	t.Helper()
	testutil.WriteFile(t, f.projectDir, "project.json", `{"name":"p","repo":"`+f.repo.Root+`","trunk":"`+trunk+`"}`)
}

// reload runs the model's reload command, its follow-up base-status loads and
// the preview of the worktree on branch cursor.
func reload(t *testing.T, m Model, cursor string) Model {
	t.Helper()
	next, _ := m.Update(cmdLoadWorktrees(m.repo, m.settings.TicketStore)())
	m = next.(Model)
	for i, wt := range m.worktrees {
		if wt.Branch == cursor {
			m.table.SetCursor(i)
		}
	}
	for _, wt := range m.worktrees {
		if m.repo.Trunk != "" {
			next, _ = m.Update(cmdLoadBaseStatus(m.repo, wt.Branch)())
			m = next.(Model)
		}
	}
	next, _ = m.Update(cmdLoadPreviewData(m.repo, *m.cursorWorktree())())
	return next.(Model)
}

func TestReload_DevelopTrunk(t *testing.T) {
	f := newTrunkFixture(t, "develop")
	m := reload(t, f.model(), "feature")

	if m.repo.Trunk != "develop" {
		t.Fatalf("Trunk = %q, want develop", m.repo.Trunk)
	}
	if m.worktrees[0].Branch != "develop" {
		t.Errorf("first row = %q, want develop", m.worktrees[0].Branch)
	}
	out := m.previewContent()
	if !strings.Contains(out, "needs rebase on develop") {
		t.Errorf("preview missing 'needs rebase on develop':\n%s", out)
	}
	if !strings.Contains(out, "1 ahead, 1 behind develop") {
		t.Errorf("preview missing ahead/behind against develop:\n%s", out)
	}
}

func TestReload_PicksUpTrunkEdit(t *testing.T) {
	f := newTrunkFixture(t, "main")
	m := reload(t, f.model(), "feature")
	if m.worktrees[0].Branch != "main" {
		t.Fatalf("first row = %q, want main", m.worktrees[0].Branch)
	}

	f.setTrunk(t, "develop")
	m = reload(t, m, "feature")

	if m.worktrees[0].Branch != "develop" {
		t.Errorf("first row after edit = %q, want develop", m.worktrees[0].Branch)
	}
	if out := m.previewContent(); !strings.Contains(out, "needs rebase on develop") {
		t.Errorf("preview after edit missing 'needs rebase on develop':\n%s", out)
	}
}

func TestReload_MissingTrunkDegrades(t *testing.T) {
	f := newTrunkFixture(t, "nope")
	m := reload(t, f.model(), "feature")

	if len(m.worktrees) != 3 {
		t.Fatalf("worktrees = %d, want 3", len(m.worktrees))
	}
	for _, wt := range m.worktrees {
		if m.isTrunk(wt.Branch) {
			t.Errorf("row %q marked as trunk", wt.Name)
		}
	}
	out := m.previewContent()
	if !strings.Contains(out, `trunk "nope" from project.json is not a local branch`) {
		t.Errorf("preview missing trunk warning:\n%s", out)
	}
	if strings.Contains(out, "rebase on") || strings.Contains(out, "rebased on") {
		t.Errorf("preview shows a base status without trunk:\n%s", out)
	}
}
