package log

import (
	"errors"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
)

func TestCmdFlashClear_NonNil(t *testing.T) {
	cmd := cmdFlashClear()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cmdFlashClear")
	}
}

func TestHandleReload_SetsRows(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", ui.Settings{})

	rows := []row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "abc", Subject: "test commit"}},
	}
	updated, _ := m.Update(reloadMsg{rows: rows})
	next := updated.(Model)
	if len(next.listPanel.Rows()) != 1 {
		t.Errorf("expected 1 row after reload, got %d", len(next.listPanel.Rows()))
	}
	if next.listPanel.Rows()[0].commit.Subject != "test commit" {
		t.Errorf("expected subject 'test commit', got %q", next.listPanel.Rows()[0].commit.Subject)
	}
}

func TestNeedsInitialLoad(t *testing.T) {
	m := newTestModel()
	if !m.NeedsInitialLoad() {
		t.Error("expected NeedsInitialLoad true before any reload (rows nil)")
	}

	rows := []row{{kind: rowCommit, commit: git.LogEntry{FullHash: "abc", Subject: "x"}}}
	updated, _ := m.Update(reloadMsg{rows: rows})
	if updated.(Model).NeedsInitialLoad() {
		t.Error("expected NeedsInitialLoad false after rows loaded")
	}
}

func TestHandleReload_Error(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(reloadMsg{err: errors.New("load failed")})
	next := updated.(Model)
	if next.err == nil {
		t.Error("expected error to be set after reload error")
	}
}

func TestHandleReload_WithFocusSubject(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", ui.Settings{})
	m.refreshing = true

	rows := []row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "a1", Subject: "first"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "b2", Subject: "target"}},
	}
	updated, cmd := m.Update(reloadMsg{rows: rows, focusSubject: "target"})
	next := updated.(Model)

	if next.listPanel.Selected() != 1 {
		t.Errorf("expected cursor at row 1 (target), got %d", next.listPanel.Selected())
	}
	if next.flashSubject != "target" {
		t.Errorf("expected flashSubject='target', got %q", next.flashSubject)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (flash clear + refresh cmds)")
	}
	if next.refreshing {
		t.Error("expected refreshing to be cleared")
	}
}

func TestHandleReload_WithRefreshing(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", ui.Settings{})
	m.refreshing = true

	updated, cmd := m.Update(reloadMsg{rows: m.listPanel.Rows()})
	next := updated.(Model)
	if next.refreshing {
		t.Error("expected refreshing=false after reload")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd from reload while refreshing")
	}
}

func TestUpdate_FlashClearMsg(t *testing.T) {
	m := newTestModel()
	m.flashSubject = "some subject"

	updated, _ := m.Update(flashClearMsg{})
	next := updated.(Model)
	if next.flashSubject != "" {
		t.Errorf("expected flashSubject cleared, got %q", next.flashSubject)
	}
}
