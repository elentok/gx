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

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P", ShiftedCode: 'P'})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected push preflight command")
	}
	updated, _ = m.Update(cmd())
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

func TestPreparePushConfirm_DivergedUsesRemoteAndUpstreamSeparately(t *testing.T) {
	repo := testutil.TempRepo(t)
	remote := t.TempDir() + "/remote.git"
	testutil.MustGitExported(t, ".", "clone", "--bare", repo, remote)
	testutil.MustGitExported(t, repo, "remote", "add", "origin", remote)
	testutil.MustGitExported(t, repo, "checkout", "-b", "feature/push")
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "feature")
	testutil.PushBranchWithUpstream(t, repo, "origin", "feature/push")

	// Diverge local from remote head.
	testutil.AmendLastCommit(t, repo)

	m := New(repo)
	m.ready = true

	if err := m.preparePushConfirm(); err != nil {
		t.Fatalf("preparePushConfirm: %v", err)
	}
	if !m.confirmOpen || m.confirmAction != confirmPushDiverged {
		t.Fatalf("expected diverged push confirm modal")
	}
	if m.confirmRemote != "origin" {
		t.Fatalf("expected force-push remote name, got %q", m.confirmRemote)
	}
	if !strings.HasPrefix(m.confirmUpstream, "origin/") {
		t.Fatalf("expected upstream ref like origin/<branch>, got %q", m.confirmUpstream)
	}
}
