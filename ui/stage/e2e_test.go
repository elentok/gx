package stage_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"gx/git"
	"gx/testutil"
	teatest "gx/testutil/teatestv2"
	"gx/ui/stage"

	tea "charm.land/bubbletea/v2"
)

const (
	stageTermWidth  = 120
	stageTermHeight = 40
	stageLoadWait   = 5 * time.Second
	stageActionWait = 3 * time.Second
)

func startStageTUI(t *testing.T, repoDir string) *teatest.TestModel {
	t.Helper()
	m := stage.New(repoDir)
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(stageTermWidth, stageTermHeight))
}

func waitForStageText(t *testing.T, tm *teatest.TestModel, text string, timeout time.Duration) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(timeout))
}

func waitForGitState(t *testing.T, tm *teatest.TestModel, timeout time.Duration, cond func() bool) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool { return cond() }, teatest.WithDuration(timeout))
}

func quitStage(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	tm.Send(keySpecial(tea.KeyEsc))
	tm.Send(keyRune('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func keySpecial(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func stagedAndUnstagedDiff(t *testing.T, repoDir, path string) (string, string) {
	t.Helper()
	staged, err := git.DiffPath(repoDir, path, true, 1)
	if err != nil {
		t.Fatalf("DiffPath(cached,%s): %v", path, err)
	}
	unstaged, err := git.DiffPath(repoDir, path, false, 1)
	if err != nil {
		t.Fatalf("DiffPath(unstaged,%s): %v", path, err)
	}
	return staged, unstaged
}

func hasAddedLine(diff, text string) bool {
	return strings.Contains(diff, "\n+"+text+"\n") || strings.HasSuffix(diff, "\n+"+text)
}

func setupAddedFileTwoHunks(t *testing.T, repoDir, path string) {
	t.Helper()
	base := strings.Join([]string{
		"old-1",
		"keep-2",
		"keep-3",
		"keep-4",
		"keep-5",
		"keep-6",
		"keep-7",
		"old-8",
	}, "\n") + "\n"
	testutil.WriteFile(t, repoDir, path, base)
	testutil.MustGitExported(t, repoDir, "add", path)

	updated := strings.Join([]string{
		"new-1",
		"keep-2",
		"keep-3",
		"keep-4",
		"keep-5",
		"keep-6",
		"keep-7",
		"new-8",
	}, "\n") + "\n"
	testutil.WriteFile(t, repoDir, path, updated)
}

func setupModifiedFileTwoHunks(t *testing.T, repoDir, path string) {
	t.Helper()
	base := strings.Join([]string{
		"old-1",
		"keep-2",
		"keep-3",
		"keep-4",
		"keep-5",
		"keep-6",
		"keep-7",
		"old-8",
	}, "\n") + "\n"
	testutil.WriteFile(t, repoDir, path, base)
	testutil.MustGitExported(t, repoDir, "add", path)
	testutil.MustGitExported(t, repoDir, "commit", "-m", "add "+path)

	updated := strings.Join([]string{
		"new-1",
		"keep-2",
		"keep-3",
		"keep-4",
		"keep-5",
		"keep-6",
		"keep-7",
		"new-8",
	}, "\n") + "\n"
	testutil.WriteFile(t, repoDir, path, updated)
}

func setupAddedFileThreeLineChanges(t *testing.T, repoDir, path string) {
	t.Helper()
	base := strings.Join([]string{"old-1", "old-2", "old-3", "keep-4", "keep-5"}, "\n") + "\n"
	testutil.WriteFile(t, repoDir, path, base)
	testutil.MustGitExported(t, repoDir, "add", path)
	updated := strings.Join([]string{"new-1", "new-2", "new-3", "keep-4", "keep-5"}, "\n") + "\n"
	testutil.WriteFile(t, repoDir, path, updated)
}

func setupModifiedFileThreeLineChanges(t *testing.T, repoDir, path string) {
	t.Helper()
	base := strings.Join([]string{"old-1", "old-2", "old-3", "keep-4", "keep-5"}, "\n") + "\n"
	testutil.WriteFile(t, repoDir, path, base)
	testutil.MustGitExported(t, repoDir, "add", path)
	testutil.MustGitExported(t, repoDir, "commit", "-m", "add "+path)
	updated := strings.Join([]string{"new-1", "new-2", "new-3", "keep-4", "keep-5"}, "\n") + "\n"
	testutil.WriteFile(t, repoDir, path, updated)
}

func TestStageE2E_StageFullNewFileFromSidebar(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "new.txt"
	testutil.WriteFile(t, repoDir, path, "a\nb\n")

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	tm.Send(keyRune(' '))

	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return staged != "" && unstaged == ""
	})

	quitStage(t, tm)
}

func TestStageE2E_StageFullModifiedFileFromSidebar(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "README.md"
	testutil.WriteFile(t, repoDir, path, "changed\n")

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	tm.Send(keyRune(' '))

	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return staged != "" && unstaged == ""
	})

	quitStage(t, tm)
}

func TestStageE2E_StageTwoHunksInNewFileFromDiffView_HunkMode(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "new-hunks.txt"
	setupAddedFileTwoHunks(t, repoDir, path)

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	tm.Send(keySpecial(tea.KeyEnter))
	tm.Send(keyRune(' '))
	tm.Send(keyRune('j'))
	tm.Send(keySpecial(tea.KeySpace))

	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return unstaged == "" && strings.Contains(staged, "new-1") && strings.Contains(staged, "new-8")
	})

	quitStage(t, tm)
}

func TestStageE2E_StageTwoHunksInModifiedFileFromDiffView_HunkMode(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "tracked-hunks.txt"
	setupModifiedFileTwoHunks(t, repoDir, path)

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	tm.Send(keySpecial(tea.KeyEnter))
	tm.Send(keySpecial(tea.KeySpace))
	tm.Send(keyRune('j'))
	tm.Send(keySpecial(tea.KeySpace))

	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return unstaged == "" && strings.Contains(staged, "new-1") && strings.Contains(staged, "new-8")
	})

	quitStage(t, tm)
}

func TestStageE2E_StageOneLineInNewFileFromDiffView_LineMode(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "new-line-1.txt"
	setupAddedFileTwoHunks(t, repoDir, path)

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	tm.Send(keySpecial(tea.KeyEnter))
	tm.Send(keyRune('a'))
	tm.Send(keyRune('j'))
	tm.Send(keyRune(' '))

	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return strings.Contains(staged, "new-1") && !strings.Contains(staged, "new-8") && strings.Contains(unstaged, "new-8")
	})

	quitStage(t, tm)
}

func TestStageE2E_StageOneLineInModifiedFileFromDiffView_LineMode(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "tracked-line-1.txt"
	setupModifiedFileTwoHunks(t, repoDir, path)

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	tm.Send(keySpecial(tea.KeyEnter))
	tm.Send(keyRune('a'))
	tm.Send(keyRune('j'))
	tm.Send(keySpecial(tea.KeySpace))

	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return strings.Contains(staged, "new-1") && !strings.Contains(staged, "new-8") && strings.Contains(unstaged, "new-8")
	})

	quitStage(t, tm)
}

func TestStageE2E_StageThirdLineInNewFileFromDiffView_LineMode(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "new-line-3.txt"
	setupAddedFileThreeLineChanges(t, repoDir, path)

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	tm.Send(keySpecial(tea.KeyEnter))
	tm.Send(keyRune('a'))
	tm.Send(keyRune('j'))
	tm.Send(keyRune('j'))
	tm.Send(keyRune('j'))
	tm.Send(keyRune('j'))
	tm.Send(keyRune('j'))
	tm.Send(keySpecial(tea.KeySpace))

	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return strings.Contains(staged, "new-3") && !strings.Contains(staged, "new-1") && !strings.Contains(staged, "new-2") && strings.Contains(unstaged, "new-1") && strings.Contains(unstaged, "new-2")
	})

	quitStage(t, tm)
}

func TestStageE2E_StageThirdLineInModifiedFileFromDiffView_LineMode(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "tracked-line-3.txt"
	setupModifiedFileThreeLineChanges(t, repoDir, path)

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	tm.Send(keySpecial(tea.KeyEnter))
	tm.Send(keyRune('a'))
	tm.Send(keyRune('j'))
	tm.Send(keyRune('j'))
	tm.Send(keyRune('j'))
	tm.Send(keyRune('j'))
	tm.Send(keyRune('j'))
	tm.Send(keySpecial(tea.KeySpace))

	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return strings.Contains(staged, "new-3") && !strings.Contains(staged, "new-1") && !strings.Contains(staged, "new-2") && strings.Contains(unstaged, "new-1") && strings.Contains(unstaged, "new-2")
	})

	quitStage(t, tm)
}

func TestStageE2E_UnstageOneHunkAfterStagingBoth_HunkMode(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "tracked-unstage-hunk.txt"
	setupModifiedFileTwoHunks(t, repoDir, path)

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	// Stage both hunks.
	tm.Send(keySpecial(tea.KeyEnter))
	tm.Send(keySpecial(tea.KeySpace))
	tm.Send(keyRune('j'))
	tm.Send(keySpecial(tea.KeySpace))

	// Unstage one hunk from staged section.
	tm.Send(keySpecial(tea.KeySpace))

	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return hasAddedLine(staged, "new-1") && !hasAddedLine(staged, "new-8") && hasAddedLine(unstaged, "new-8")
	})

	quitStage(t, tm)
}

func TestStageE2E_UnstageOneLineAfterStagingAll_LineMode(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	path := "added-unstage-line.txt"
	setupAddedFileThreeLineChanges(t, repoDir, path)

	tm := startStageTUI(t, repoDir)
	waitForStageText(t, tm, path, stageLoadWait)

	// Stage everything from status view first.
	tm.Send(keyRune(' '))
	waitForGitState(t, tm, stageActionWait, func() bool {
		staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return staged != "" && unstaged == ""
	})

	// Enter diff view and unstage just one line.
	tm.Send(keySpecial(tea.KeyEnter))
	waitForStageText(t, tm, "diff: mode:hunk", stageActionWait)
	tm.Send(keyRune('a'))
	waitForStageText(t, tm, "diff: mode:line", stageActionWait)
	tm.Send(keyRune('j'))
	tm.Send(keyRune(' '))

	waitForGitState(t, tm, stageActionWait, func() bool {
		_, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
		return unstaged != ""
	})

	staged, unstaged := stagedAndUnstagedDiff(t, repoDir, path)
	if !hasAddedLine(staged, "new-1") || hasAddedLine(staged, "new-2") || !hasAddedLine(staged, "new-3") || !hasAddedLine(unstaged, "new-2") {
		t.Fatalf("unexpected line unstage result\nSTAGED:\n%s\nUNSTAGED:\n%s", staged, unstaged)
	}

	quitStage(t, tm)
}
