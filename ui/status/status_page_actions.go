package status

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) handleActionResult(res stageActionResult) tea.Cmd {
	if strings.TrimSpace(res.output) != "" {
		m.recordCommandOutput(stageActionOutputTitle(res.kind), res.output)
	}
	if res.promptPopStash {
		m.openConfirm("Pop stash after rebase failure?", []string{"The rebase failed after stashing changes.", "Pop the stash now?"}, confirmRebasePopStash, "", "")
		m.runningOpen = false
		return nil
	}
	if res.err != nil {
		m.showGitError(fmt.Errorf("%w\n%s", res.err, strings.TrimSpace(res.output)))
		m.runningOpen = false
		return nil
	}
	switch res.kind {
	case actionRebase:
		m.setStatus(ui.MessageComplete("rebase"))
	case actionPopStashRebase:
		m.setStatus("stash restored")
	case actionAmend:
		m.setStatus("amended last commit")
	}
	m.runningOpen = false
	return m.refresh()
}

func (m Model) confirmAccept() (tea.Model, tea.Cmd) {
	if !m.confirmYes {
		m.confirmOpen = false
		m.confirmAction = confirmNone
		return m, nil
	}
	a := m.confirmAction
	m.confirmOpen = false
	m.confirmAction = confirmNone

	switch a {
	case confirmRebase:
		titleTarget := strings.TrimSpace(m.confirmRemote)
		if titleTarget == "" {
			titleTarget = detectRebaseTarget(m.worktreeRoot)
		}
		runner := newStageActionRunner(actionRebase, m.worktreeRoot, titleTarget, m.confirmBranch)
		m.openRunning("Rebase on "+titleTarget, runner)
		return m, actionPollCmd()
	case confirmRebasePopStash:
		runner := newStageActionRunner(actionPopStashRebase, m.worktreeRoot, "", "")
		m.openRunning("Pop stash", runner)
		return m, actionPollCmd()
	case confirmAmend:
		runner := newStageActionRunner(actionAmend, m.worktreeRoot, "", "")
		m.openRunning("Amend commit", runner)
		return m, actionPollCmd()
	case confirmDiscardStatus:
		if m.confirmDiscardUntracked {
			if err := git.DiscardUntrackedPath(m.worktreeRoot, m.confirmPaths[0]); err != nil {
				m.showGitError(err)
				return m, nil
			}
		} else {
			if err := git.RestorePaths(m.worktreeRoot, m.confirmPaths); err != nil {
				m.showGitError(err)
				return m, nil
			}
		}
		m.setStatus("discarded " + m.confirmPaths[0])
		return m, m.reload(m.confirmPaths[0])
	case confirmDiscardUnstaged:
		if err := git.ApplyPatchToWorktree(m.worktreeRoot, m.confirmPatch, true, m.confirmPatchUnidiffZero); err != nil {
			m.showGitError(err)
			return m, nil
		}
		if m.diffarea.ActiveSection == diffarea.SectionUnstaged {
			m.diffarea.DisableVisual()
		}
		m.setStatus("discarded " + m.confirmPaths[0])
		cmd := m.reload(m.confirmPaths[0])
		if m.focus == focusDiff {
			m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
		}
		return m, cmd
	}
	return m, nil
}

func (m *Model) prepareRebaseConfirm() error {
	branch, err := git.CurrentBranch(m.worktreeRoot)
	if err != nil {
		return err
	}
	if branch == "" || branch == "HEAD" {
		return fmt.Errorf("cannot rebase: detached HEAD")
	}
	target := detectRebaseTarget(m.worktreeRoot)
	m.openConfirm(
		fmt.Sprintf("Rebase branch %s on %s?", branch, target),
		nil,
		confirmRebase,
		target,
		branch,
	)
	return nil
}

func detectRebaseTarget(root string) string {
	repo, err := git.FindRepo(root)
	if err != nil {
		return "origin/main"
	}
	main := strings.TrimSpace(repo.MainBranch)
	if main == "" {
		main = "main"
	}
	return "origin/" + main
}

func (m *Model) openAmendConfirm() error {
	subject, files, err := amendPreview(m.worktreeRoot)
	if err != nil {
		return err
	}
	lines := []string{fmt.Sprintf("Commit: %s", subject), "", "Files in last commit:"}
	limit := 10
	if len(files) < limit {
		limit = len(files)
	}
	for i := 0; i < limit; i++ {
		lines = append(lines, "- "+files[i])
	}
	if len(files) > limit {
		lines = append(lines, "...")
	}
	m.openConfirm("Amend last commit?", lines, confirmAmend, "", "")
	return nil
}

func amendPreview(root string) (subject string, files []string, err error) {
	out, _, err := execGitCapture(root, "log", "-1", "--pretty=%s")
	if err != nil {
		return "", nil, err
	}
	nameOut, _, err := execGitCapture(root, "show", "--name-only", "--pretty=format:", "HEAD")
	if err != nil {
		return "", nil, err
	}
	for _, line := range strings.Split(nameOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, line)
	}
	return strings.TrimSpace(out), files, nil
}

func execGitCapture(root string, args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return stdout, stderr, &git.RunError{Args: args, Dir: root, Stdout: stdout, Stderr: stderr, Code: ee.ExitCode()}
		}
		return stdout, stderr, err
	}
	return stdout, stderr, nil
}
