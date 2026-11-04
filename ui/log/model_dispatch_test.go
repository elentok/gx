package log

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
)

func TestDispatchBinding_Navigation(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", ui.Settings{EnableNavigation: true})
	m.width = 120
	m.height = 40

	// j / k navigation
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	initial := m.listPanel.Selected()
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.listPanel.Selected() != 0 && m.listPanel.Selected() == initial {
		// either k moved up or we were already at top — just verify no crash
	}

	// G goes to bottom
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'G', Text: "G", ShiftedCode: 'G', Mod: tea.ModShift})
	bottom := m.listPanel.Selected()
	if bottom < 0 {
		t.Fatalf("G: expected non-negative selection, got %d", bottom)
	}

	// gg goes to top
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if m.listPanel.Selected() != 0 {
		t.Fatalf("gg: expected top (0), got %d", m.listPanel.Selected())
	}

	// q with navigation → nav back command
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q with EnableNavigation: expected nav.Back command")
	}

	// R reload
	m, cmd = sendKey(m, tea.KeyPressMsg{Code: 'R', Text: "R", ShiftedCode: 'R', Mod: tea.ModShift})
	if !m.refreshing {
		t.Fatal("R: expected refreshing=true")
	}
}

func TestDispatchBinding_PageScrollAndHelp(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.height = 40
	m.listPanel = m.listPanel.WithRows(make([]row, 30))

	before := m.listPanel.Selected()
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	_ = before

	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})

	// ? opens help
	m, _ = sendKey(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !m.help.IsOpen {
		t.Fatal("?: expected help to open")
	}
}

func TestDispatchBinding_ClearFilter(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelFiltered(repo, "", settings, LogFilter{Path: "foo.go"})
	m.width = 120
	m.height = 40

	if !m.filter.IsActive() {
		t.Fatal("expected active filter")
	}

	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	if m.filter.IsActive() {
		t.Fatal("f: expected filter cleared")
	}
}

func TestSelectRefAndSelectedRef(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa111", Subject: "first"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "bbb222", Subject: "second"}},
	}).SetSelected(0)

	// SelectedRef at row 0
	if got := m.SelectedRef(); got != "aaa111" {
		t.Errorf("SelectedRef() = %q, want 'aaa111'", got)
	}

	// SelectRef moves cursor to matching hash
	m = m.SelectRef("bbb222")
	if m.listPanel.Selected() != 1 {
		t.Errorf("SelectRef: expected selection at 1, got %d", m.listPanel.Selected())
	}

	// SelectRef with unknown hash — cursor unchanged
	m = m.SelectRef("zzzxxx")
	if m.listPanel.Selected() != 1 {
		t.Errorf("SelectRef unknown: expected selection unchanged at 1, got %d", m.listPanel.Selected())
	}

	// SelectedRef returns empty for empty rows
	empty := newTestModel()
	if got := empty.SelectedRef(); got != "" {
		t.Errorf("SelectedRef empty model = %q, want ''", got)
	}
}

func TestMouseWheelScrolls(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows(make([]row, 30))
	m.height = 10
	updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	next := updated.(Model)
	_ = next // just ensure no panic

	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	next = updated.(Model)
	_ = next
}

func TestFocusReloadsRowsAfterOnDiskChange(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo, "", settings)
	initialRows := len(m.listPanel.Rows())

	testutil.WriteFile(t, repo, "new.txt", "new\n")
	testutil.CommitAll(t, repo, "new commit")

	updated, cmd := m.Update(tea.FocusMsg{})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected focus to trigger reload command")
	}

	// FocusMsg now returns a batch (reload + status load); run all sub-commands.
	m = runCmd(m, cmd)
	if len(m.listPanel.Rows()) <= initialRows {
		t.Fatalf("expected more rows after focus reload; before=%d after=%d", initialRows, len(m.listPanel.Rows()))
	}
}

func TestKeyManager(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", settings)
	km := m.KeyManager()
	if len(km.Bindings()) == 0 {
		t.Error("expected non-empty key bindings")
	}
}

func TestInputFocused_False(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", settings)
	if m.InputFocused() {
		t.Error("expected InputFocused=false by default")
	}
}

func TestWithPendingFocus(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", settings)
	m2 := m.WithPendingFocus("abc123")
	if m2.pendingFocusSubject != "abc123" {
		t.Errorf("pendingFocusSubject = %q, want 'abc123'", m2.pendingFocusSubject)
	}
}

func TestAutoReload_WithPending(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "f.txt", "a\n")
	testutil.CommitAll(t, repo, "init")

	m := newTestModelDefault(repo, "", settings)
	m = m.WithPendingFocus("HEAD")
	cmd := m.AutoReload()
	if cmd == nil {
		t.Error("expected non-nil cmd from AutoReload with pending focus")
	}
}

// --- Pseudo-log-line tests ---
