package status

import (
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"

	tea "charm.land/bubbletea/v2"
)

func TestPushKeyOpensPushModel(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "init")
	testutil.MustGitExported(t, repo, "checkout", "-b", "feature/test")

	m := New(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P", ShiftedCode: 'P'})
	m = updated.(Model)

	if !m.push.IsOpen {
		t.Fatalf("expected push key to open push.Model")
	}
}

func TestRebaseKeyOpensSpecificConfirm(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "init")
	testutil.MustGitExported(t, repo, "checkout", "-b", "feature/test")

	m := New(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(Model)

	if !m.confirmOpen {
		t.Fatalf("expected rebase key to open confirmation")
	}
	if !strings.Contains(m.confirmTitle, "Rebase branch feature/test on origin/main?") {
		t.Fatalf("unexpected rebase confirm title: %q", m.confirmTitle)
	}
}

func TestAppendRunningOutputStripsTerminalControlSequences(t *testing.T) {
	m := New(testutil.TempRepo(t))
	m.ready = true
	m.openRunning("Amend", newStageActionRunner(actionAmend, m.worktreeRoot, "", ""))

	m.appendRunningOutput("\x1b[32mWriting objects: 100%\x1b[0m\r\n")
	m.appendRunningOutput("remote: \x1b]8;;https://example.com\x07https://example.com\x1b]8;;\x07\n")

	if strings.Contains(m.runningContent, "\x1b") {
		t.Fatalf("expected running content to strip ANSI escapes, got %q", m.runningContent)
	}
	if strings.Contains(m.runningContent, "\r") {
		t.Fatalf("expected running content to strip carriage returns, got %q", m.runningContent)
	}
	if !strings.Contains(m.runningContent, "Writing objects: 100%") {
		t.Fatalf("expected sanitized progress text, got %q", m.runningContent)
	}
	if !strings.Contains(m.runningContent, "remote: https://example.com") {
		t.Fatalf("expected sanitized hyperlink text, got %q", m.runningContent)
	}
}
