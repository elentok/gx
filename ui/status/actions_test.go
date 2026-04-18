package stage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"

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
	if cmd != nil {
		t.Fatalf("expected no preflight command before confirmation")
	}

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

func TestStageActionRunnerAccumulatesCompositePullOutput(t *testing.T) {
	repo := testutil.TempBareRepoWithMainWorktreeAhead(t)
	mainWt := filepath.Join(repo, "main")
	testutil.WriteFile(t, mainWt, "README.md", "modified")

	runner := newStageActionRunner(actionPull, mainWt, "", "")
	res := runner.runPullLike(actionPull)
	res.output = runner.log.String()

	if res.err != nil {
		t.Fatalf("run pull action: %v\n%s", res.err, res.output)
	}
	for _, want := range []string{"$ git stash push -u -m gx-stage-auto-stash", "$ git pull", "$ git stash pop"} {
		if !strings.Contains(res.output, want) {
			t.Fatalf("expected output to include %q, got:\n%s", want, res.output)
		}
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
	if !m.confirmOpen || m.confirmAction != confirmPush {
		t.Fatalf("expected initial push confirmation")
	}

	updated, cmd := m.confirmAccept()
	m = updated.(Model)
	if cmd == nil || m.runningRunner == nil {
		t.Fatalf("expected push fetch runner after confirming push")
	}

	m.handleActionResult(stageActionResult{
		kind:   actionPushFetch,
		branch: m.confirmBranch,
		remote: m.confirmRemote,
		output: "$ git fetch origin\n(no output)",
	})

	if !m.confirmOpen || m.confirmAction != confirmPushDiverged {
		t.Fatalf("expected diverged push confirm modal after preflight")
	}
	if m.confirmRemote != "origin" {
		t.Fatalf("expected force-push remote name, got %q", m.confirmRemote)
	}
	if !strings.HasPrefix(m.confirmUpstream, "origin/") {
		t.Fatalf("expected upstream ref like origin/<branch>, got %q", m.confirmUpstream)
	}
}

func TestAppendRunningOutputStripsTerminalControlSequences(t *testing.T) {
	m := New(testutil.TempRepo(t))
	m.ready = true
	m.openRunning("Push", newStageActionRunner(actionPush, m.worktreeRoot, "origin", "main"))

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
