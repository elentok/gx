package log

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
)

func TestPseudoLogLineIsFirstRow(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "first commit")

	m := newTestModelDefault(repo, "", settings)
	rows := m.listPanel.Rows()
	if len(rows) == 0 {
		t.Fatal("expected rows after init")
	}
	if rows[0].kind != rowPseudoStatus {
		t.Fatalf("rows[0].kind = %v, want rowPseudoStatus", rows[0].kind)
	}
}

func TestPseudoLogLineLoadingState(t *testing.T) {
	// Before the worktree status is loaded, the pseudo-line shows "loading".
	m := newTestModel()
	r := row{kind: rowPseudoStatus, detail: ""}
	m.listPanel = m.listPanel.WithRows([]row{r})
	lines := m.listPanel.WithHints(m.buildHints()).renderRow(r, false, 80)
	stripped := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(stripped, "working tree") {
		t.Errorf("renderRow: expected 'working tree' label, got %q", stripped)
	}
}

func TestPseudoLogLineCleanState(t *testing.T) {
	m := newTestModel()
	m.statusLoaded = true
	// staged=0, unstaged=0, untracked=0 → "no local changes"
	r := row{kind: rowPseudoStatus, detail: m.pseudoStatusDetail()}
	m.listPanel = m.listPanel.WithRows([]row{r})
	lines := m.listPanel.WithHints(m.buildHints()).renderRow(r, false, 80)
	stripped := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(stripped, "no local changes") {
		t.Errorf("renderRow: expected 'no local changes', got %q", stripped)
	}
}

func TestPseudoLogLineDirtyState(t *testing.T) {
	m := newTestModel()
	m.statusLoaded = true
	m.statusStaged = 2
	m.statusUnstaged = 3
	m.statusUntracked = 1
	r := row{kind: rowPseudoStatus, detail: m.pseudoStatusDetail()}
	m.listPanel = m.listPanel.WithRows([]row{r})
	lines := m.listPanel.WithHints(m.buildHints()).renderRow(r, false, 80)
	stripped := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(stripped, "2 staged") {
		t.Errorf("expected '2 staged' in %q", stripped)
	}
	if !strings.Contains(stripped, "3 unstaged") {
		t.Errorf("expected '3 unstaged' in %q", stripped)
	}
	if !strings.Contains(stripped, "1 untracked") {
		t.Errorf("expected '1 untracked' in %q", stripped)
	}
}

func TestPseudoLogLineDirtyStateZeroCountsOmitted(t *testing.T) {
	m := newTestModel()
	m.statusLoaded = true
	m.statusStaged = 1
	// unstaged=0, untracked=0 — these counts must be absent from output.
	r := row{kind: rowPseudoStatus, detail: m.pseudoStatusDetail()}
	m.listPanel = m.listPanel.WithRows([]row{r})
	detail := m.pseudoStatusDetail()
	if strings.Contains(detail, "0 ") {
		t.Errorf("detail should omit zero counts, got %q", detail)
	}
	if !strings.Contains(detail, "1 staged") {
		t.Errorf("expected '1 staged' in detail %q", detail)
	}
}

func TestEnterOnPseudoLogLineEmitsStatusSwitch(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", ui.Settings{EnableNavigation: true})
	// Move cursor to the pseudo-line (index 0).
	m.listPanel = m.listPanel.SetSelected(0)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter on pseudo-log-line")
	}
	msg := cmd()
	vs, ok := nav.IsSwitch(msg)
	if !ok {
		t.Fatalf("expected nav.Switch message, got %T", msg)
	}
	if vs.Tab != nav.TabStatus {
		t.Errorf("expected TabStatus, got %q", vs.Tab)
	}
	if vs.WorktreeRoot != repo {
		t.Errorf("expected WorktreeRoot=%q, got %q", repo, vs.WorktreeRoot)
	}
}
