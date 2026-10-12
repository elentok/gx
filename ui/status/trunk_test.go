package status

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/notify"
)

// pressRebase registers a repo on branch feature/test with trunk and presses
// the rebase key.
func pressRebase(t *testing.T, trunk string) (Model, tea.Cmd) {
	t.Helper()
	repoDir := rebaseRepo(t)
	testutil.RegisterProject(t, repoDir, trunk)
	return pressRebaseIn(repoDir)
}

// rebaseRepo is a repo on branch feature/test with a local develop branch and
// origin/develop.
func rebaseRepo(t *testing.T) string {
	t.Helper()
	repoDir := testutil.TempRepo(t)
	testutil.MustGitExported(t, repoDir, "branch", "develop")
	testutil.MustGitExported(t, repoDir, "update-ref", "refs/remotes/origin/develop", "HEAD")
	testutil.MustGitExported(t, repoDir, "checkout", "-b", "feature/test")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	return repo.Root
}

func pressRebaseIn(repoDir string) (Model, tea.Cmd) {
	m := newTestModelDefault(repoDir)
	m.ready = true
	m.focus = focusFiletree
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	return updated.(Model), cmd
}

func TestRebaseTargetFollowsTrunk(t *testing.T) {
	m, _ := pressRebase(t, "develop")
	if !strings.Contains(m.confirmTitle, "on origin/develop?") {
		t.Fatalf("confirm title = %q, want rebase on origin/develop", m.confirmTitle)
	}
}

func TestRebaseTargetLocalOnlyTrunk(t *testing.T) {
	repoDir := rebaseRepo(t)
	testutil.MustGitExported(t, repoDir, "branch", "local-trunk", "develop")
	testutil.RegisterProject(t, repoDir, "local-trunk")
	m, _ := pressRebaseIn(repoDir)
	if !strings.Contains(m.confirmTitle, "on local-trunk?") {
		t.Fatalf("confirm title = %q, want rebase on local-trunk", m.confirmTitle)
	}
}

func TestRebaseUnreadableProjectFileWarns(t *testing.T) {
	repoDir := rebaseRepo(t)
	projectDir := testutil.RegisterProject(t, repoDir, "develop")
	testutil.WriteFile(t, projectDir, "project.json", "{not json")
	m, cmd := pressRebaseIn(repoDir)
	assertWarning(t, m, cmd, "project.json")
}

func TestRebaseMissingTrunkWarnsWithoutFallback(t *testing.T) {
	m, cmd := pressRebase(t, "nope")
	assertWarning(t, m, cmd, `"nope"`)
}

func assertWarning(t *testing.T, m Model, cmd tea.Cmd, want string) {
	t.Helper()
	if m.confirmOpen {
		t.Fatalf("want no confirm, got %q", m.confirmTitle)
	}
	if cmd == nil {
		t.Fatal("want a warning")
	}
	msg, ok := cmd().(notify.NotifyMsg)
	if !ok || msg.Kind != notify.KindWarning || !strings.Contains(msg.Message, want) {
		t.Fatalf("got %#v, want a warning containing %q", msg, want)
	}
}
