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
	repoDir := testutil.TempRepo(t)
	testutil.MustGitExported(t, repoDir, "branch", "develop")
	testutil.MustGitExported(t, repoDir, "checkout", "-b", "feature/test")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	testutil.RegisterProject(t, repo.Root, trunk)

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

func TestRebaseMissingTrunkWarnsWithoutFallback(t *testing.T) {
	m, cmd := pressRebase(t, "nope")
	if m.confirmOpen {
		t.Fatalf("want no confirm, got %q", m.confirmTitle)
	}
	if cmd == nil {
		t.Fatal("want a warning")
	}
	msg, ok := cmd().(notify.NotifyMsg)
	if !ok || msg.Kind != notify.KindWarning || !strings.Contains(msg.Message, `"nope"`) {
		t.Fatalf("got %#v, want a trunk warning", msg)
	}
}
