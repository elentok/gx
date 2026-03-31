package stage

import (
	"strings"
	"testing"

	"gx/testutil"

	tea "charm.land/bubbletea/v2"
)

func TestPushKeyOpensSpecificConfirm(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "init")
	testutil.MustGitExported(t, repo, "checkout", "-b", "feature/test")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P", ShiftedCode: 'P'})
	m = updated.(Model)

	if !m.confirmOpen {
		t.Fatalf("expected push key to open confirmation")
	}
	if !strings.Contains(m.confirmTitle, "Push branch feature/test to origin?") {
		t.Fatalf("unexpected push confirm title: %q", m.confirmTitle)
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
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(Model)

	if !m.confirmOpen {
		t.Fatalf("expected rebase key to open confirmation")
	}
	if !strings.Contains(m.confirmTitle, "Rebase branch feature/test on origin/main?") {
		t.Fatalf("unexpected rebase confirm title: %q", m.confirmTitle)
	}
}

func TestPushResultWithPRURLOpensConfirm(t *testing.T) {
	m := New(testutil.TempRepo(t))
	m.ready = true

	m.handleActionResult(stageActionResult{kind: actionPush, prURL: "https://github.com/org/repo/pull/new/feature"})

	if !m.confirmOpen {
		t.Fatalf("expected PR URL confirmation to open")
	}
	if !strings.Contains(m.confirmTitle, "Open pull request page?") || !strings.Contains(m.confirmTitle, "/pull/new/") {
		t.Fatalf("unexpected PR confirm prompt: %q", m.confirmTitle)
	}
	if !m.confirmYes {
		t.Fatalf("expected PR confirm to default to yes")
	}
}
