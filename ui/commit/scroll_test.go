package commit

import (
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/keys"
	ui "github.com/elentok/gx/ui"
)

func newScrollTestModel(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "file.txt", "a\n")
	testutil.CommitAll(t, repo, "init")
	m := NewModel(repo, "HEAD", "", ui.Settings{}, keys.Manager{})
	m.ready = true
	m.width = 120
	m.height = 40
	return m
}

func TestScrollDiffPage_NoContent(t *testing.T) {
	m := newScrollTestModel(t)
	m.scrollDiffPage(1) // should not panic
}

func TestJumpDiffTop_NoContent(t *testing.T) {
	m := newScrollTestModel(t)
	m.jumpDiffTop() // should not panic
}

func TestJumpDiffBottom_NoContent(t *testing.T) {
	m := newScrollTestModel(t)
	m.jumpDiffBottom() // should not panic
}

func TestJumpSidebarTop_Empty(t *testing.T) {
	m := newScrollTestModel(t)
	moved := m.jumpSidebarTop()
	if moved {
		t.Error("expected jumpSidebarTop=false with empty filetree")
	}
}

func TestJumpSidebarBottom_Empty(t *testing.T) {
	m := newScrollTestModel(t)
	moved := m.jumpSidebarBottom()
	if moved {
		t.Error("expected jumpSidebarBottom=false with empty filetree")
	}
}

func TestScrollSidebarPage_NoContent(t *testing.T) {
	m := newScrollTestModel(t)
	m.scrollSidebarPage(1) // should not panic
}
