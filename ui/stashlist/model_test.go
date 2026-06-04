package stashlist

import (
	"os/exec"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
)

// runInit executes Init() synchronously and feeds the result back to Update.
func runInit(m Model) Model {
	cmd := m.Init()
	if cmd == nil {
		return m
	}
	msg := cmd()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

// send sends a message to the model and returns the updated Model.
func send(m Model, msg tea.Msg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func mustStashFile(t *testing.T, dir, name string) {
	t.Helper()
	testutil.WriteFile(t, dir, name+".txt", "changed\n")
	for _, args := range [][]string{
		{"add", name + ".txt"},
		{"stash", "push", "-m", name},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestEmptyStashListRendersNoStashes(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := runInit(NewModel(repo))

	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	content := m.View().Content
	if !containsVisible(content, "no stashes") {
		t.Fatalf("expected 'no stashes' in output, got:\n%s", content)
	}
}

func TestSingleEntryRendersRefAndMessage(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	mustStashFile(t, repo, "my-stash")

	m := runInit(NewModel(repo))
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	content := m.View().Content

	if !containsVisible(content, "stash@{0}") {
		t.Fatalf("expected stash ref in output, got:\n%s", content)
	}
	if !containsVisible(content, "my-stash") {
		t.Fatalf("expected stash message in output, got:\n%s", content)
	}
}

func TestNavigationChangesSelection(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	mustStashFile(t, repo, "stash-a")
	mustStashFile(t, repo, "stash-b")

	m := runInit(NewModel(repo))
	if m.list.Selected() != 0 {
		t.Fatalf("expected selection 0, got %d", m.list.Selected())
	}
	if m.SelectedRef() != "stash@{0}" {
		t.Fatalf("expected stash@{0}, got %q", m.SelectedRef())
	}

	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = send(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.list.Selected() != 1 {
		t.Fatalf("expected selection 1 after j, got %d", m.list.Selected())
	}
	if m.SelectedRef() != "stash@{1}" {
		t.Fatalf("expected stash@{1}, got %q", m.SelectedRef())
	}

	m = send(m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.list.Selected() != 0 {
		t.Fatalf("expected selection 0 after k, got %d", m.list.Selected())
	}
}

func TestSelectionEmitsCorrectRef(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	mustStashFile(t, repo, "first")
	mustStashFile(t, repo, "second")

	m := runInit(NewModel(repo))
	// newest stash is stash@{0}, oldest is stash@{1}
	if got := m.SelectedRef(); got != "stash@{0}" {
		t.Fatalf("initial ref = %q, want stash@{0}", got)
	}

	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = send(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if got := m.SelectedRef(); got != "stash@{1}" {
		t.Fatalf("after j ref = %q, want stash@{1}", got)
	}
}

func TestContainerFocusControlsFrameColors(t *testing.T) {
	t.Parallel()

	active := NewModel("").WithContainerFocus(true)
	if active.frameBorderColor() != ui.ColorOrange {
		t.Fatalf("active border color = %v, want %v", active.frameBorderColor(), ui.ColorOrange)
	}
	if active.frameTitleColor() != ui.ColorOrange {
		t.Fatalf("active title color = %v, want %v", active.frameTitleColor(), ui.ColorOrange)
	}

	inactive := NewModel("").WithContainerFocus(false)
	if inactive.frameBorderColor() != ui.ColorBorder {
		t.Fatalf("inactive border color = %v, want %v", inactive.frameBorderColor(), ui.ColorBorder)
	}
	if inactive.frameTitleColor() != ui.ColorMauve {
		t.Fatalf("inactive title color = %v, want %v", inactive.frameTitleColor(), ui.ColorMauve)
	}
}

// containsVisible strips ANSI escapes and checks for a substring.
func containsVisible(s, sub string) bool {
	return len(s) > 0 && contains(ansi.Strip(s), sub)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := range len(s) - len(sub) + 1 {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
