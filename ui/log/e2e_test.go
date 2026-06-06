package log_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/log"
)

const (
	logE2ETermWidth  = 120
	logE2ETermHeight = 40
	logE2ELoadWait   = 5 * time.Second
	logE2EActionWait = 15 * time.Second
)

func startLogTUI(t *testing.T, repoDir string) *teatest.TestModel {
	t.Helper()
	m := log.NewModel(repoDir, "", ui.Settings{}, log.LogFilter{}, keys.Manager{})
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(logE2ETermWidth, logE2ETermHeight))
}

func startCommitViewFromLog(t *testing.T, repoDir, ref string) *teatest.TestModel {
	t.Helper()
	m := commit.NewModel(repoDir, ref, "", ui.Settings{}, keys.Manager{})
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(logE2ETermWidth, logE2ETermHeight))
}

func waitForLogE2EText(t *testing.T, tm *teatest.TestModel, text string, timeout time.Duration) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(timeout))
}

func waitForLogE2ETexts(t *testing.T, tm *teatest.TestModel, timeout time.Duration, texts ...string) {
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

func logE2EKeyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func logE2EKeySpecial(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func logE2EKeyCtrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

func logE2EGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func logE2EFindCommitHash(t *testing.T, dir, subject string) string {
	t.Helper()
	out := logE2EGitOutput(t, dir, "log", "--format=%H %s")
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && parts[1] == subject {
			return parts[0]
		}
	}
	t.Fatalf("commit with subject %q not found in log", subject)
	return ""
}

// waitForLogHashChange polls git until the commit with subject has a different hash.
func waitForLogHashChange(t *testing.T, tm *teatest.TestModel, repoDir, subject, oldHash string) {
	t.Helper()
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
				return true
			}
		}
		return false
	}, teatest.WithDuration(logE2EActionWait))
}

// waitForStashPop waits for file to appear in unstaged changes, indicating the
// post-rebase stash-pop background step has finished.
func waitForStashPop(t *testing.T, tm *teatest.TestModel, repoDir, file string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool {
		cmd := exec.Command("git", "diff", "--name-only")
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), file)
	}, teatest.WithDuration(logE2EActionWait))
}

// logE2EQuit uses ctrl+c because q sends nav.Back() (handled by the app shell, not the model).
func logE2EQuit(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	tm.Send(logE2EKeyCtrl('c'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// verifyAmendedNonHEADCommit checks git state and opens a commit view to confirm the amended content.
func verifyAmendedNonHEADCommit(t *testing.T, repoDir, middleHashBefore string) {
	t.Helper()

	middleHashAfter := logE2EFindCommitHash(t, repoDir, "middle")
	if middleHashAfter == middleHashBefore {
		t.Fatal("middle commit hash did not change after amend")
	}

	// a.txt diff in the amended commit should contain the staged change
	aDiff := logE2EGitOutput(t, repoDir, "show", middleHashAfter, "--", "a.txt")
	if !strings.Contains(aDiff, "+a amended") {
		t.Errorf("expected '+a amended' in a.txt diff; got:\n%s", aDiff)
	}

	// b.txt in the amended commit should have its original content, not the unstaged change
	bDiff := logE2EGitOutput(t, repoDir, "show", middleHashAfter, "--", "b.txt")
	if strings.Contains(bDiff, "+b changed") {
		t.Errorf("b.txt in amended commit should not contain unstaged change; got:\n%s", bDiff)
	}

	// b.txt and c.txt should still be unstaged in the working tree (stash was popped)
	unstagedFiles := logE2EGitOutput(t, repoDir, "diff", "--name-only")
	if !strings.Contains(unstagedFiles, "b.txt") {
		t.Errorf("b.txt should have unstaged changes after amend; unstaged:\n%s", unstagedFiles)
	}
	if !strings.Contains(unstagedFiles, "c.txt") {
		t.Errorf("c.txt should have unstaged changes after amend; unstaged:\n%s", unstagedFiles)
	}

	// Open the amended commit in a commit view and verify a.txt appears in the file list
	commitTM := startCommitViewFromLog(t, repoDir, middleHashAfter)
	waitForLogE2EText(t, commitTM, "a.txt", logE2ELoadWait)
	logE2EQuit(t, commitTM)
}

func TestAmendE2E_NonHEAD_FromLog(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepoWithThreeCommits(t)
	middleHashBefore := logE2EFindCommitHash(t, repoDir, "middle")

	testutil.WriteFile(t, repoDir, "a.txt", "a amended\n")
	testutil.WriteFile(t, repoDir, "b.txt", "b changed but not staged\n")
	testutil.WriteFile(t, repoDir, "c.txt", "c changed but not staged\n")
	testutil.MustGitExported(t, repoDir, "add", "a.txt")

	tm := startLogTUI(t, repoDir)

	// Wait for log to load with HEAD "tip" at top
	waitForLogE2EText(t, tm, "tip", logE2ELoadWait)

	// Navigate down to "middle" and open the amend modal
	tm.Send(logE2EKeyRune('j'))
	tm.Send(logE2EKeyRune('A'))

	// Confirm modal shows commit info and the staged file
	waitForLogE2ETexts(t, tm, logE2EActionWait, "Amend staged changes into:", "middle", "a.txt")

	// Accept and wait for the git hash to change (amend complete)
	tm.Send(logE2EKeySpecial(tea.KeyEnter))
	waitForLogHashChange(t, tm, repoDir, "middle", middleHashBefore)

	waitForStashPop(t, tm, repoDir, "b.txt")
	logE2EQuit(t, tm)

	verifyAmendedNonHEADCommit(t, repoDir, middleHashBefore)
}

func TestAmendE2E_HEAD_FromLog(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepoWithThreeCommits(t)
	headHashBefore := logE2EGitOutput(t, repoDir, "rev-parse", "HEAD")

	// Stage a change to c.txt (added in "tip", which is HEAD)
	testutil.WriteFile(t, repoDir, "c.txt", "c amended\n")
	testutil.MustGitExported(t, repoDir, "add", "c.txt")

	tm := startLogTUI(t, repoDir)

	// "tip" (HEAD) is at top and selected by default — no navigation needed
	waitForLogE2EText(t, tm, "tip", logE2ELoadWait)

	tm.Send(logE2EKeyRune('A'))

	// Confirm modal
	waitForLogE2ETexts(t, tm, logE2EActionWait, "Amend staged changes into:", "tip", "c.txt")

	// Accept and wait for the git hash to change (amend complete)
	tm.Send(logE2EKeySpecial(tea.KeyEnter))
	waitForLogHashChange(t, tm, repoDir, "tip", headHashBefore)

	logE2EQuit(t, tm)

	headHashAfter := logE2EGitOutput(t, repoDir, "rev-parse", "HEAD")
	if headHashAfter == headHashBefore {
		t.Fatal("HEAD hash did not change after amend")
	}

	files := logE2EGitOutput(t, repoDir, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "c.txt") {
		t.Errorf("HEAD commit should contain c.txt; got:\n%s", files)
	}

	diff := logE2EGitOutput(t, repoDir, "show", "HEAD", "--", "c.txt")
	if !strings.Contains(diff, "+c amended") {
		t.Errorf("expected '+c amended' in diff; got:\n%s", diff)
	}

	status := logE2EGitOutput(t, repoDir, "status", "--porcelain")
	if status != "" {
		t.Errorf("expected clean working tree after HEAD amend; status:\n%s", status)
	}

	// Open the new HEAD in a commit view and verify c.txt is shown
	commitTM := startCommitViewFromLog(t, repoDir, headHashAfter)
	waitForLogE2EText(t, commitTM, "c.txt", logE2ELoadWait)
	logE2EQuit(t, commitTM)
}

func TestOpenCommitDetailFromLog(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepoWithThreeCommits(t)

	tm := startLogTUI(t, repoDir)

	// Wait for log to load — "tip" is HEAD and should appear first
	waitForLogE2EText(t, tm, "tip", logE2ELoadWait)

	// Press Enter on "tip" to open the split-view detail panel
	tm.Send(logE2EKeySpecial(tea.KeyEnter))

	// The commit detail should show the file added in "tip"
	waitForLogE2EText(t, tm, "c.txt", logE2EActionWait)

	logE2EQuit(t, tm)
}

// waitForCurrentFrameContaining polls CurrentFrame until the latest rendered
// frame contains text, or fails after timeout.
func waitForCurrentFrameContaining(t *testing.T, tm *teatest.TestModel, text string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if bytes.Contains(tm.CurrentFrame(), []byte(text)) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected %q in current frame after %v", text, timeout)
}

// waitForCurrentFrameWithout polls CurrentFrame until the latest rendered
// frame no longer contains text, or fails after timeout.
func waitForCurrentFrameWithout(t *testing.T, tm *teatest.TestModel, text string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !bytes.Contains(tm.CurrentFrame(), []byte(text)) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected %q to be absent from current frame after %v", text, timeout)
}

// TestSplitViewPanelSwitchingE2E covers the enter→esc→esc navigation cycle:
//  1. enter opens the split detail panel (detail focused)
//  2. first esc moves focus back to the list (split stays open)
//  3. second esc collapses the split (detail panel disappears)
func TestSplitViewPanelSwitchingE2E(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepoWithThreeCommits(t)
	tm := startLogTUI(t, repoDir)

	// Wait for the log to load; "tip" (HEAD) is the first real commit.
	waitForLogE2EText(t, tm, "tip", logE2ELoadWait)

	// Open the split detail panel on the selected commit.
	tm.Send(logE2EKeySpecial(tea.KeyEnter))
	// The detail panel renders with the "Commit" frame title.
	waitForLogE2EText(t, tm, "Commit", logE2EActionWait)

	// First esc: focus moves from detail back to the list; split stays open.
	tm.Send(logE2EKeySpecial(tea.KeyEsc))
	// Navigate down in the list — once "middle" appears in the current frame
	// we know the model has processed the esc and the list has focus.
	tm.Send(logE2EKeyRune('j'))
	waitForCurrentFrameContaining(t, tm, "middle", logE2EActionWait)
	if !bytes.Contains(tm.CurrentFrame(), []byte("Commit")) {
		t.Fatal("expected 'Commit' panel still visible after first esc (should only defocus, not collapse)")
	}

	// Second esc: collapses the split — the detail panel must disappear.
	tm.Send(logE2EKeySpecial(tea.KeyEsc))
	waitForCurrentFrameWithout(t, tm, "Commit", logE2EActionWait)

	logE2EQuit(t, tm)
}
