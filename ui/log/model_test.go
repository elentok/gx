package log

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/splitview"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var settings = ui.Settings{}

func newTestModel() Model {
	m := Model{
		keys:   newLogManager(),
		search: search.NewModel(),
	}
	m.help = help.NewModel(help.BuildSections(m.keys, m.search.Keys()), nil)
	m.listPanel = newListPanel()
	m.split = splitview.New(m.listPanel, m.commitDetail)
	return m
}

func newTestModelDefault(worktreeRoot, startRef string, settings ui.Settings) Model {
	return runModelInit(NewModel(worktreeRoot, startRef, settings, LogFilter{}, keys.Manager{}))
}

func newTestModelFiltered(worktreeRoot, startRef string, settings ui.Settings, filter LogFilter) Model {
	return runModelInit(NewModel(worktreeRoot, startRef, settings, filter, keys.Manager{}))
}

// runModelInit executes Init() synchronously so tests can inspect m.rows immediately.
func runModelInit(m Model) Model {
	return runCmd(m, m.Init())
}

// runCmd executes a single command synchronously, handling batch commands
// by running each sub-command in sequence.
func runCmd(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runCmd(m, c)
		}
		return m
	}
	next, _ := m.Update(msg)
	if m2, ok := next.(Model); ok {
		return m2
	}
	return m
}

func sendKey(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestWorktreeStatusUpdatesRows(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowPseudoStatus, detail: "loading worktree status…"},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1"}},
	})

	updated, _ := m.Update(worktreeStatusMsg{staged: 1, unstaged: 2, untracked: 0})
	m = updated.(Model)

	if !m.statusLoaded {
		t.Error("expected statusLoaded=true")
	}
	detail := m.listPanel.Rows()[0].detail
	if !strings.Contains(detail, "1 staged") {
		t.Errorf("expected '1 staged' in detail %q", detail)
	}
	if !strings.Contains(detail, "2 unstaged") {
		t.Errorf("expected '2 unstaged' in detail %q", detail)
	}
	if strings.Contains(detail, "untracked") {
		t.Errorf("expected no 'untracked' for count=0, got %q", detail)
	}
}

func TestEnterOnCommitRendersDetailContent(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a content\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)

	// WindowSizeMsg must go through Update to set m.ready = true (same as real app).
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	m = updated.(Model)
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatal("expected split mode after Enter")
	}

	view := m.View()
	stripped := ansi.Strip(view.Content)
	if !strings.Contains(stripped, "a.txt") {
		t.Errorf("expected 'a.txt' in view after opening commit detail, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "Files") {
		t.Errorf("expected 'Files' pane in view after opening commit detail, got:\n%s", stripped)
	}
}

func TestFlashOnJumpSetsFlashState(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa111", Subject: "target commit"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "bbb222", Subject: "other commit"}},
	})

	updated, _ := m.Update(reloadMsg{
		rows: []row{
			{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa111", Subject: "target commit"}},
			{kind: rowCommit, commit: git.LogEntry{FullHash: "bbb222", Subject: "other commit"}},
		},
		focusSubject: "target commit",
	})
	m = updated.(Model)

	if m.flashSubject != "target commit" {
		t.Errorf("flashSubject = %q, want 'target commit'", m.flashSubject)
	}
	if m.flashUntil.IsZero() {
		t.Error("flashUntil should be set after flash-on-jump")
	}
	if m.listPanel.Selected() != 0 {
		t.Errorf("cursor should be on matched row 0, got %d", m.listPanel.Selected())
	}
}

func TestFlashClearMsgResetsFlashState(t *testing.T) {
	m := newTestModel()
	m.flashSubject = "some subject"
	m.flashUntil = time.Now().Add(2 * time.Second)

	updated, _ := m.Update(flashClearMsg{})
	m = updated.(Model)

	if m.flashSubject != "" {
		t.Errorf("flashSubject should be cleared, got %q", m.flashSubject)
	}
	if !m.flashUntil.IsZero() {
		t.Error("flashUntil should be zero after clear")
	}
}

func TestFilteredLogShowsOnlyFilteredCommits(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit touching a")
	testutil.WriteFile(t, repo, "b.txt", "b\n")
	testutil.CommitAll(t, repo, "commit touching b only")

	m := newTestModelFiltered(repo, "", settings, LogFilter{Path: "b.txt"})
	rows := m.listPanel.Rows()
	for _, r := range rows {
		if r.kind != rowCommit {
			continue
		}
		if r.commit.Subject == "commit touching a" {
			t.Fatal("filtered log should not contain commits that don't touch b.txt")
		}
	}
	found := false
	for _, r := range rows {
		if r.kind == rowCommit && r.commit.Subject == "commit touching b only" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("filtered log should contain commit that touches b.txt")
	}
}
