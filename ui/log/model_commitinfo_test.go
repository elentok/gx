package log

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"

	tea "charm.land/bubbletea/v2"
)

func TestCmdFetchCommitInfo_EmptyRows(t *testing.T) {
	m := newTestModel()
	cmd := m.cmdFetchCommitInfo()
	if cmd != nil {
		t.Error("expected nil cmd when no rows")
	}
}

func TestCmdFetchCommitInfo_PseudoStatusRow(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{{kind: rowPseudoStatus, label: "status"}}).SetSelected(0)
	cmd := m.cmdFetchCommitInfo()
	if cmd != nil {
		t.Error("expected nil cmd for pseudo-status row")
	}
}

func TestHandleCommitInfo_OpensPopupWithSelectedCommit(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", ui.Settings{})
	m.width, m.height = 100, 40

	var hash string
	for i, r := range m.listPanel.Rows() {
		if r.kind == rowCommit {
			m.listPanel = m.listPanel.SetSelected(i)
			hash = r.commit.FullHash
			break
		}
	}
	if hash == "" {
		t.Skip("no commit rows in test repo")
	}

	cmd := m.cmdFetchCommitInfo()
	if cmd == nil {
		t.Fatal("expected non-nil cmd for commit row")
	}
	msg := cmd()
	infoMsg, ok := msg.(commitInfoMsg)
	if !ok {
		t.Fatalf("expected commitInfoMsg, got %T", msg)
	}
	if infoMsg.err != nil {
		t.Fatalf("unexpected fetch error: %v", infoMsg.err)
	}

	next, _ := m.handleCommitInfo(infoMsg)
	updated := next.(Model)
	if !updated.commitInfoOpen {
		t.Fatal("expected commitInfoOpen to be true")
	}
	if updated.commitInfoDetails.FullHash != hash {
		t.Errorf("expected popup for hash %s, got %s", hash, updated.commitInfoDetails.FullHash)
	}

	out := updated.renderCommitInfoPopup()
	if !strings.Contains(out, updated.commitInfoDetails.Subject) {
		t.Error("expected rendered popup to contain commit subject")
	}
}

func TestHandleCommitInfoKey_EscClosesPopup(t *testing.T) {
	m := newTestModel()
	m.commitInfoOpen = true
	next, _ := m.handleCommitInfoKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated := next.(Model)
	if updated.commitInfoOpen {
		t.Fatal("expected esc to close the commit info popup")
	}
}

func TestHandleCommitInfoKey_IClosesPopup(t *testing.T) {
	m := newTestModel()
	m.commitInfoOpen = true
	next, _ := m.handleCommitInfoKey(tea.KeyPressMsg{Code: 'i', Text: "i"})
	updated := next.(Model)
	if updated.commitInfoOpen {
		t.Fatal("expected i to close the commit info popup")
	}
}

func TestHandleCommitInfoKey_OtherKeyStaysOpen(t *testing.T) {
	m := newTestModel()
	m.commitInfoOpen = true
	next, _ := m.handleCommitInfoKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	updated := next.(Model)
	if !updated.commitInfoOpen {
		t.Fatal("expected non-closing key to leave the popup open")
	}
}

func TestRenderCommitInfoPopup_HasBorder(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 30
	m.commitInfoDetails = git.CommitDetails{
		FullHash: "abc123", Hash: "abc123", Subject: "a subject", AuthorName: "Jane",
	}
	out := m.renderCommitInfoPopup()
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╮") ||
		!strings.Contains(out, "╰") || !strings.Contains(out, "╯") {
		t.Errorf("expected bordered output, got:\n%s", out)
	}
}
