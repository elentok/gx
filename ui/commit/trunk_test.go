package commit

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
)

// deliverTrunk runs cmd (unwrapping tea.BatchMsg) and feeds any
// TrunkLoadedMsg it yields back into the model.
func deliverTrunk(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = deliverTrunk(t, m, c)
		}
	case TrunkLoadedMsg:
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	return m
}

func TestTrunkResolvesAsync(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	testutil.MustGitExported(t, repoDir, "branch", "develop")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	projectDir, store := testutil.RegisterProject(t, repo.Root, "develop")

	m := newTestModelWithFilter(repoDir, "HEAD", "", ui.Settings{UseNerdFontIcons: true, TicketStore: store})
	if m.trunk != "" {
		t.Fatalf("trunk = %q before the load cmd ran, want none", m.trunk)
	}
	m = deliverTrunk(t, m, m.Init())
	if m.trunk != "develop" {
		t.Fatalf("trunk = %q, want develop", m.trunk)
	}

	testutil.WriteFile(t, projectDir, "project.json", `{"name":"p","repo":"`+repo.Root+`","trunk":"nope"}`)
	m, cmd := m.WithRef("HEAD")
	m = deliverTrunk(t, m, cmd)
	if m.trunk != "" {
		t.Fatalf("trunk = %q after it went missing, want none", m.trunk)
	}
}
