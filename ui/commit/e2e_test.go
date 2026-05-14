package commit_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui/commit"
)

const (
	commitE2ETermWidth  = 120
	commitE2ETermHeight = 40
	commitE2ELoadWait   = 5 * time.Second
	commitE2EActionWait = 15 * time.Second
)

func startCommitTUI(t *testing.T, repoDir, ref string) *teatest.TestModel {
	t.Helper()
	m := commit.NewWithSettings(repoDir, ref, commit.Settings{})
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(commitE2ETermWidth, commitE2ETermHeight))
}

func waitForCommitE2EText(t *testing.T, tm *teatest.TestModel, text string, timeout time.Duration) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(timeout))
}

func waitForCommitE2ETexts(t *testing.T, tm *teatest.TestModel, timeout time.Duration, texts ...string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		for _, text := range texts {
			if !bytes.Contains(bts, []byte(text)) {
				return false
			}
		}
		return true
	}, teatest.WithDuration(timeout))
}

func commitE2EKeyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func commitE2EKeySpecial(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func commitE2EKeyCtrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

func commitE2EGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitE2EFindCommitHash(t *testing.T, dir, subject string) string {
	t.Helper()
	out := commitE2EGitOutput(t, dir, "log", "--format=%H %s")
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && parts[1] == subject {
			return parts[0]
		}
	}
	t.Fatalf("commit with subject %q not found in log", subject)
	return ""
}

// waitForCommitHashChange polls git until the commit at subject has a different hash.
// Uses tm.Output() as a trigger so the spinner drives the polling rate.
func waitForCommitHashChange(t *testing.T, tm *teatest.TestModel, repoDir, subject, oldHash string) string {
	t.Helper()
	var newHash string
	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool {
		cmd := exec.Command("git", "log", "--format=%H %s")
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return false
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 && parts[1] == subject && parts[0] != oldHash {
				newHash = parts[0]
				return true
			}
		}
		return false
	}, teatest.WithDuration(commitE2EActionWait))
	return newHash
}

// commitE2EQuit uses ctrl+c because q sends nav.Back() (handled by the app shell, not the model).
func commitE2EQuit(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	tm.Send(commitE2EKeyCtrl('c'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestAmendE2E_NonHEAD_FromCommitView(t *testing.T) {
	repoDir := testutil.TempRepoWithThreeCommits(t)
	middleHashBefore := commitE2EFindCommitHash(t, repoDir, "middle")

	testutil.WriteFile(t, repoDir, "a.txt", "a amended\n")
	testutil.WriteFile(t, repoDir, "b.txt", "b changed but not staged\n")
	testutil.WriteFile(t, repoDir, "c.txt", "c changed but not staged\n")
	testutil.MustGitExported(t, repoDir, "add", "a.txt")

	tm := startCommitTUI(t, repoDir, middleHashBefore)

	// Wait for commit view to load showing the middle commit's files
	waitForCommitE2ETexts(t, tm, commitE2ELoadWait, "middle", "a.txt")

	tm.Send(commitE2EKeyRune('A'))

	// Confirm modal
	waitForCommitE2ETexts(t, tm, commitE2EActionWait, "Amend staged changes into:", "middle", "a.txt")

	// Accept and poll git until the middle commit hash changes
	tm.Send(commitE2EKeySpecial(tea.KeyEnter))
	middleHashAfter := waitForCommitHashChange(t, tm, repoDir, "middle", middleHashBefore)

	commitE2EQuit(t, tm)

	// a.txt diff in the amended commit should contain the staged change
	aDiff := commitE2EGitOutput(t, repoDir, "show", middleHashAfter, "--", "a.txt")
	if !strings.Contains(aDiff, "+a amended") {
		t.Errorf("expected '+a amended' in a.txt diff; got:\n%s", aDiff)
	}

	// b.txt in the amended commit should have its original content, not the unstaged change
	bDiff := commitE2EGitOutput(t, repoDir, "show", middleHashAfter, "--", "b.txt")
	if strings.Contains(bDiff, "+b changed") {
		t.Errorf("b.txt in amended commit should not contain unstaged change; got:\n%s", bDiff)
	}

	// b.txt and c.txt should still be unstaged in the working tree (stash was popped)
	unstagedFiles := commitE2EGitOutput(t, repoDir, "diff", "--name-only")
	if !strings.Contains(unstagedFiles, "b.txt") {
		t.Errorf("b.txt should have unstaged changes after amend; unstaged:\n%s", unstagedFiles)
	}
	if !strings.Contains(unstagedFiles, "c.txt") {
		t.Errorf("c.txt should have unstaged changes after amend; unstaged:\n%s", unstagedFiles)
	}

	// Open the amended commit in a new commit view and verify a.txt is shown
	verifyTM := startCommitTUI(t, repoDir, middleHashAfter)
	waitForCommitE2EText(t, verifyTM, "a.txt", commitE2ELoadWait)
	commitE2EQuit(t, verifyTM)
}

func TestAmendE2E_HEAD_FromCommitView(t *testing.T) {
	repoDir := testutil.TempRepoWithThreeCommits(t)
	headHashBefore := commitE2EGitOutput(t, repoDir, "rev-parse", "HEAD")

	// Stage a change to c.txt (added in "tip", which is HEAD)
	testutil.WriteFile(t, repoDir, "c.txt", "c amended\n")
	testutil.MustGitExported(t, repoDir, "add", "c.txt")

	tm := startCommitTUI(t, repoDir, "HEAD")

	// Wait for commit view to load
	waitForCommitE2ETexts(t, tm, commitE2ELoadWait, "tip", "c.txt")

	tm.Send(commitE2EKeyRune('A'))

	// Confirm modal
	waitForCommitE2ETexts(t, tm, commitE2EActionWait, "Amend staged changes into:", "tip", "c.txt")

	// Accept and poll git until HEAD changes
	tm.Send(commitE2EKeySpecial(tea.KeyEnter))
	var headHashAfter string
	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool {
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return false
		}
		h := strings.TrimSpace(string(out))
		if h != headHashBefore {
			headHashAfter = h
			return true
		}
		return false
	}, teatest.WithDuration(commitE2EActionWait))

	commitE2EQuit(t, tm)

	files := commitE2EGitOutput(t, repoDir, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "c.txt") {
		t.Errorf("HEAD commit should contain c.txt; got:\n%s", files)
	}

	diff := commitE2EGitOutput(t, repoDir, "show", "HEAD", "--", "c.txt")
	if !strings.Contains(diff, "+c amended") {
		t.Errorf("expected '+c amended' in diff; got:\n%s", diff)
	}

	status := commitE2EGitOutput(t, repoDir, "status", "--porcelain")
	if status != "" {
		t.Errorf("expected clean working tree after HEAD amend; status:\n%s", status)
	}

	// Open the new HEAD in a commit view and verify c.txt is shown
	verifyTM := startCommitTUI(t, repoDir, headHashAfter)
	waitForCommitE2EText(t, verifyTM, "c.txt", commitE2ELoadWait)
	commitE2EQuit(t, verifyTM)
}

func TestAmendE2E_Conflict_FromCommitView(t *testing.T) {
	repoDir := testutil.TempRepoWithConflictSetup(t)
	middleHash := commitE2EFindCommitHash(t, repoDir, "middle")

	// Stage a conflicting change to a.txt
	testutil.WriteFile(t, repoDir, "a.txt", "conflicting change\n")
	testutil.MustGitExported(t, repoDir, "add", "a.txt")

	tm := startCommitTUI(t, repoDir, middleHash)

	// Wait for commit view to load
	waitForCommitE2ETexts(t, tm, commitE2ELoadWait, "middle", "a.txt")

	tm.Send(commitE2EKeyRune('A'))

	// Confirm modal
	waitForCommitE2ETexts(t, tm, commitE2EActionWait, "Amend staged changes into:", "middle", "a.txt")

	// Accept — fixup step succeeds, rebase step fails with conflict
	tm.Send(commitE2EKeySpecial(tea.KeyEnter))

	// Wait for the modal to show both the done fixup step and the failed rebase step
	waitForCommitE2ETexts(t, tm, commitE2EActionWait, "created fixup commit", "rebase failed")

	// Dismiss
	tm.Send(commitE2EKeySpecial(tea.KeyEsc))

	commitE2EQuit(t, tm)

	// Repo should be left in a mid-rebase conflicted state
	status := commitE2EGitOutput(t, repoDir, "status", "--porcelain")
	if !strings.Contains(status, "AA a.txt") && !strings.Contains(status, "UU a.txt") {
		t.Errorf("expected conflict markers for a.txt after failed rebase; status:\n%s", status)
	}
}
