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
	cmd := m.commitInfo.fetch(m.worktreeRoot, m.listPanel.Rows(), m.listPanel.Selected())
	if cmd != nil {
		t.Error("expected nil cmd when no rows")
	}
}

func TestCmdFetchCommitInfo_PseudoStatusRow(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{{kind: rowPseudoStatus, label: "status"}}).SetSelected(0)
	cmd := m.commitInfo.fetch(m.worktreeRoot, m.listPanel.Rows(), m.listPanel.Selected())
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

	cmd := m.commitInfo.fetch(m.worktreeRoot, m.listPanel.Rows(), m.listPanel.Selected())
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

	m.commitInfo.handle(infoMsg)
	updated := m
	if !updated.commitInfo.open {
		t.Fatal("expected commitInfo.open to be true")
	}
	if updated.commitInfo.details.FullHash != hash {
		t.Errorf("expected popup for hash %s, got %s", hash, updated.commitInfo.details.FullHash)
	}

	out := updated.commitInfo.render(updated.width, updated.height)
	if !strings.Contains(out, updated.commitInfo.details.Subject) {
		t.Error("expected rendered popup to contain commit subject")
	}
}

func TestHandleCommitInfoKey_EscClosesPopup(t *testing.T) {
	m := newTestModel()
	m.commitInfo.open = true
	m.commitInfo.key(tea.KeyPressMsg{Code: tea.KeyEscape}, m.width, m.height)
	updated := m
	if updated.commitInfo.open {
		t.Fatal("expected esc to close the commit info popup")
	}
}

func TestHandleCommitInfoKey_IClosesPopup(t *testing.T) {
	m := newTestModel()
	m.commitInfo.open = true
	m.commitInfo.key(tea.KeyPressMsg{Code: 'i', Text: "i"}, m.width, m.height)
	updated := m
	if updated.commitInfo.open {
		t.Fatal("expected i to close the commit info popup")
	}
}

func TestHandleCommitInfoKey_OtherKeyStaysOpen(t *testing.T) {
	m := newTestModel()
	m.commitInfo.open = true
	m.commitInfo.key(tea.KeyPressMsg{Code: 'j', Text: "j"}, m.width, m.height)
	updated := m
	if !updated.commitInfo.open {
		t.Fatal("expected non-closing key to leave the popup open")
	}
}

func TestHandleCommitInfoKey_ScrollsBodyWithLongCommit(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	m.commitInfo.open = true
	body := strings.Repeat("line\n", 40)
	m.commitInfo.details = git.CommitDetails{FullHash: "abc123", Hash: "abc123", Subject: "subject", Body: body}

	m.commitInfo.key(tea.KeyPressMsg{Code: 'j', Text: "j"}, m.width, m.height)
	updated := m
	if updated.commitInfo.scroll != 1 {
		t.Fatalf("expected j to scroll popup down by 1, got %d", updated.commitInfo.scroll)
	}

	updated.commitInfo.key(tea.KeyPressMsg{Code: 'k', Text: "k"}, updated.width, updated.height)
	if updated.commitInfo.scroll != 0 {
		t.Fatalf("expected k to scroll popup back up to 0, got %d", updated.commitInfo.scroll)
	}
}

func TestHandleCommitInfoWheel_ScrollsPopupNotListBehind(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	m.commitInfo.open = true
	body := strings.Repeat("line\n", 40)
	m.commitInfo.details = git.CommitDetails{FullHash: "abc123", Hash: "abc123", Subject: "subject", Body: body}
	m.listPanel = m.listPanel.WithRows(commitRows(30))
	prevListOffset := m.listPanel.list.Offset()

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	updated := next.(Model)

	if updated.commitInfo.scroll != ui.WheelScrollLines {
		t.Fatalf("expected wheel-down to scroll popup by %d, got %d", ui.WheelScrollLines, updated.commitInfo.scroll)
	}
	if updated.listPanel.list.Offset() != prevListOffset {
		t.Fatalf("expected log list offset unchanged while commit-info popup open, got %d want %d", updated.listPanel.list.Offset(), prevListOffset)
	}
}

func TestRenderCommitInfoPopup_HasBorder(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 30
	m.commitInfo.details = git.CommitDetails{
		FullHash: "abc123", Hash: "abc123", Subject: "a subject", AuthorName: "Jane",
	}
	out := m.commitInfo.render(m.width, m.height)
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╮") ||
		!strings.Contains(out, "╰") || !strings.Contains(out, "╯") {
		t.Errorf("expected bordered output, got:\n%s", out)
	}
}
