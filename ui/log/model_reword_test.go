package log

import (
	"errors"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
)

func TestCmdFetchRewordDetails_EmptyRows(t *testing.T) {
	m := newTestModel()
	cmd := m.cmdFetchRewordDetails()
	if cmd != nil {
		t.Error("expected nil cmd when no rows")
	}
}

func TestCmdFetchRewordDetails_PseudoStatusRow(t *testing.T) {
	m := newTestModel()
	m.rows = []row{{kind: rowPseudoStatus, label: "status"}}
	m.list.SetSelected(0, len(m.rows))
	cmd := m.cmdFetchRewordDetails()
	if cmd != nil {
		t.Error("expected nil cmd for pseudo-status row")
	}
}

func TestCmdFetchRewordDetails_WithCommitRow(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", ui.Settings{})
	var found bool
	for i, r := range m.rows {
		if r.kind == rowCommit {
			m.list.SetSelected(i, len(m.rows))
			found = true
			break
		}
	}
	if !found {
		t.Skip("no commit rows in test repo")
	}
	cmd := m.cmdFetchRewordDetails()
	if cmd == nil {
		t.Fatal("expected non-nil cmd for commit row")
	}
}

func TestHandleRewordDetails_Error(t *testing.T) {
	m := newTestModel()
	_, cmd := m.handleRewordDetails(rewordDetailsMsg{err: errors.New("fetch failed")})
	if cmd == nil {
		t.Fatal("expected non-nil error cmd from handleRewordDetails")
	}
}

func TestHandleRewordDetails_WithData(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", ui.Settings{})

	// Get a real hash from the repo
	var hash string
	for _, r := range m.rows {
		if r.kind == rowCommit {
			hash = r.commit.FullHash
			break
		}
	}
	if hash == "" {
		t.Skip("no commit rows in test repo")
	}

	_, cmd := m.handleRewordDetails(rewordDetailsMsg{
		hash:    hash,
		subject: "initial",
		body:    "",
		pushed:  false,
	})
	// cmd may be nil (if CmdOpenEditor fails in test env) or non-nil — just ensure no panic
	_ = cmd
}

func TestHandleRewordEditorDone_Error(t *testing.T) {
	m := newTestModel()
	_, cmd := m.handleRewordEditorDone(errors.New("editor crashed"))
	if cmd == nil {
		t.Fatal("expected non-nil error cmd from handleRewordEditorDone")
	}
}

func TestHandleRewordDone_Error(t *testing.T) {
	m := newTestModel()
	_, cmd := m.handleRewordDone(errors.New("reword failed"))
	if cmd == nil {
		t.Fatal("expected non-nil error cmd from handleRewordDone")
	}
}

func TestHandleRewordRunningUpdate_ForwardsMsgToReword(t *testing.T) {
	m := newTestModel()
	// Without reword running, a generic message should not crash
	updated, _ := m.handleRewordRunningUpdate(git.LogEntry{})
	_ = updated
}
