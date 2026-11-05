package status

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/tickets/ticketgit"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/status/diffarea"
)

func (m *Model) handleActionResult(res stageActionResult) tea.Cmd {
	if strings.TrimSpace(res.output) != "" {
		m.recordCommandOutput(stageActionOutputTitle(res.kind), res.output)
	}
	if res.promptPopStash {
		m.openConfirm("Pop stash after rebase failure?", []string{"The rebase failed after stashing changes.", "Pop the stash now?"}, confirmRebasePopStash, "", "")
		m.modals.runningOpen = false
		return nil
	}
	if res.err != nil {
		m.showGitError(fmt.Errorf("%w\n%s", res.err, strings.TrimSpace(res.output)))
		m.modals.runningOpen = false
		return nil
	}
	var notifyCmd tea.Cmd
	switch res.kind {
	case actionRebase:
		notifyCmd = notify.Success(ui.MessageComplete("rebase"))
	case actionPopStashRebase:
		notifyCmd = notify.Info("stash restored")
	case actionAmend:
		notifyCmd = notify.Success("amended last commit")
	}
	m.modals.runningOpen = false
	return tea.Batch(notifyCmd, m.refresh())
}

// confirmAccept runs the action of a confirm request the modal closed with an
// accepted answer.
func (m Model) confirmAccept(req confirmRequest) (Model, tea.Cmd) {
	if !req.yes {
		return m, nil
	}

	switch req.action {
	case confirmRebase:
		titleTarget := strings.TrimSpace(req.remote)
		if titleTarget == "" {
			var err error
			if titleTarget, err = detectRebaseTarget(m.settings.TicketStore, m.worktreeRoot); err != nil {
				return m, notify.Warning(err.Error())
			}
		}
		runner := newStageActionRunner(actionRebase, m.worktreeRoot, titleTarget, req.branch)
		runner.store = m.settings.TicketStore
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
		if req.discardUntracked {
			if err := stage.DiscardUntrackedPath(m.worktreeRoot, req.paths[0]); err != nil {
				m.showGitError(err)
				return m, nil
			}
		} else {
			if err := stage.RestorePaths(m.worktreeRoot, req.paths); err != nil {
				m.showGitError(err)
				return m, nil
			}
		}
		if len(req.deletePaths) > 0 {
			for _, path := range req.deletePaths {
				if err := stage.DiscardUntrackedPath(m.worktreeRoot, path); err != nil {
					m.showGitError(err)
					return m, nil
				}
			}
		}
		displayPath := req.displayPath
		if displayPath == "" && len(req.paths) > 0 {
			displayPath = req.paths[0]
		}
		preservePath := req.preservePath
		if preservePath == "" {
			preservePath = displayPath
		}
		return m, tea.Batch(notify.Success("discarded "+displayPath), m.reload(preservePath))
	case confirmDiscardUnstaged:
		if err := stage.ApplyPatchToWorktree(m.worktreeRoot, req.patch, true, req.patchUnidiffZero); err != nil {
			m.showGitError(err)
			return m, nil
		}
		if m.diffarea.ActiveSection == diffarea.SectionUnstaged {
			m.diffarea.DisableVisual()
		}
		cmd := m.reload(req.paths[0])
		if m.focus == focusDiff {
			m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
		}
		return m, tea.Batch(notify.Success("discarded "+req.paths[0]), cmd)
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
	target, err := detectRebaseTarget(m.settings.TicketStore, m.worktreeRoot)
	if err != nil {
		return rebaseTargetError{err}
	}
	m.openConfirm(
		fmt.Sprintf("Rebase branch %s on %s?", branch, target),
		nil,
		confirmRebase,
		target,
		branch,
	)
	return nil
}

// rebaseTargetError marks a trunk-resolution failure, which the status view
// shows as a warning rather than a git error.
type rebaseTargetError struct{ error }

func (e rebaseTargetError) Unwrap() error { return e.error }

// detectRebaseTarget resolves trunk on every call, so a project.json edit
// applies at once. A missing trunk is an error, never a guessed branch.
func detectRebaseTarget(store, root string) (string, error) {
	repo, err := ticketgit.OpenRepoIn(store, root)
	if err != nil {
		return "", err
	}
	return git.TrunkBaseRef(repo.Root, repo.Trunk), nil
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
	stdout, stderr, err := git.Run(root, git.NonInteractiveEnv(), args...)
	return strings.TrimSpace(stdout), strings.TrimSpace(stderr), err
}
