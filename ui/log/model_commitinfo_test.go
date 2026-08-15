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

func TestHandleCommitInfoKey_ScrollsBodyWithLongCommit(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	m.commitInfoOpen = true
	body := strings.Repeat("line\n", 40)
	m.commitInfoDetails = git.CommitDetails{FullHash: "abc123", Hash: "abc123", Subject: "subject", Body: body}

	next, _ := m.handleCommitInfoKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	updated := next.(Model)
	if updated.commitInfoScroll != 1 {
		t.Fatalf("expected j to scroll popup down by 1, got %d", updated.commitInfoScroll)
	}

	next, _ = updated.handleCommitInfoKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	updated = next.(Model)
	if updated.commitInfoScroll != 0 {
		t.Fatalf("expected k to scroll popup back up to 0, got %d", updated.commitInfoScroll)
	}
}

func TestHandleCommitInfoWheel_ScrollsPopupNotListBehind(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	m.commitInfoOpen = true
	body := strings.Repeat("line\n", 40)
	m.commitInfoDetails = git.CommitDetails{FullHash: "abc123", Hash: "abc123", Subject: "subject", Body: body}
	m.listPanel = m.listPanel.WithRows(commitRows(30))
	prevListOffset := m.listPanel.list.Offset()

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	updated := next.(Model)

	if updated.commitInfoScroll != ui.WheelScrollLines {
		t.Fatalf("expected wheel-down to scroll popup by %d, got %d", ui.WheelScrollLines, updated.commitInfoScroll)
	}
	if updated.listPanel.list.Offset() != prevListOffset {
		t.Fatalf("expected log list offset unchanged while commit-info popup open, got %d want %d", updated.listPanel.list.Offset(), prevListOffset)
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
