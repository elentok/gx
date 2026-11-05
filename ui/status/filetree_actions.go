package status

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/status/diffarea"
	"github.com/elentok/gx/ui/status/stagemodal"
)

func handleActionResult(m *Model, res stagemodal.ActionResult) tea.Cmd {
	if strings.TrimSpace(res.Output) != "" {
		recordCommandOutput(m, stagemodal.OutputTitle(res.Kind), res.Output)
	}
	if res.PromptPopStash {
		openConfirm(m, "Pop stash after rebase failure?", []string{"The rebase failed after stashing changes.", "Pop the stash now?"}, stagemodal.ConfirmRebasePopStash, "", "")
		m.modals.CloseRunning()
		return nil
	}
	if res.Err != nil {
		m.showGitError(fmt.Errorf("%w\n%s", res.Err, strings.TrimSpace(res.Output)))
		m.modals.CloseRunning()
		return nil
	}
	var notifyCmd tea.Cmd
	switch res.Kind {
	case stagemodal.ActionRebase:
		notifyCmd = notify.Success(ui.MessageComplete("rebase"))
	case stagemodal.ActionPopStashRebase:
		notifyCmd = notify.Info("stash restored")
	case stagemodal.ActionAmend:
		notifyCmd = notify.Success("amended last commit")
	}
	m.modals.CloseRunning()
	return tea.Batch(notifyCmd, m.refresh())
}

// confirmAccept runs the action of a confirm request the modal closed with an
// accepted answer.
func confirmAccept(m Model, req stagemodal.ConfirmRequest) (Model, tea.Cmd) {
	if !req.Yes {
		return m, nil
	}

	switch req.Action {
	case stagemodal.ConfirmRebase:
		titleTarget := strings.TrimSpace(req.Remote)
		if titleTarget == "" {
			var err error
			if titleTarget, err = stagemodal.DetectRebaseTarget(m.settings.TicketStore, m.worktreeRoot); err != nil {
				return m, notify.Warning(err.Error())
			}
		}
		runner := stagemodal.NewRunner(stagemodal.ActionRebase, m.worktreeRoot, titleTarget, req.Branch)
		runner.Store = m.settings.TicketStore
		openRunning(&m, "Rebase on "+titleTarget, runner)
		return m, stagemodal.PollCmd()
	case stagemodal.ConfirmRebasePopStash:
		runner := stagemodal.NewRunner(stagemodal.ActionPopStashRebase, m.worktreeRoot, "", "")
		openRunning(&m, "Pop stash", runner)
		return m, stagemodal.PollCmd()
	case stagemodal.ConfirmAmend:
		runner := stagemodal.NewRunner(stagemodal.ActionAmend, m.worktreeRoot, "", "")
		openRunning(&m, "Amend commit", runner)
		return m, stagemodal.PollCmd()
	case stagemodal.ConfirmDiscardStatus:
		if req.DiscardUntracked {
			if err := stage.DiscardUntrackedPath(m.worktreeRoot, req.Paths[0]); err != nil {
				m.showGitError(err)
				return m, nil
			}
		} else {
			if err := stage.RestorePaths(m.worktreeRoot, req.Paths); err != nil {
				m.showGitError(err)
				return m, nil
			}
		}
		if len(req.DeletePaths) > 0 {
			for _, path := range req.DeletePaths {
				if err := stage.DiscardUntrackedPath(m.worktreeRoot, path); err != nil {
					m.showGitError(err)
					return m, nil
				}
			}
		}
		displayPath := req.DisplayPath
		if displayPath == "" && len(req.Paths) > 0 {
			displayPath = req.Paths[0]
		}
		preservePath := req.PreservePath
		if preservePath == "" {
			preservePath = displayPath
		}
		return m, tea.Batch(notify.Success("discarded "+displayPath), m.reload(preservePath))
	case stagemodal.ConfirmDiscardUnstaged:
		if err := stage.ApplyPatchToWorktree(m.worktreeRoot, req.Patch, true, req.PatchUnidiffZero); err != nil {
			m.showGitError(err)
			return m, nil
		}
		if m.diffarea.ActiveSection == diffarea.SectionUnstaged {
			m.diffarea.DisableVisual()
		}
		cmd := m.reload(req.Paths[0])
		if m.focus == focusDiff {
			m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
		}
		return m, tea.Batch(notify.Success("discarded "+req.Paths[0]), cmd)
	}
	return m, nil
}

func prepareRebaseConfirm(m *Model) error {
	branch, err := git.CurrentBranch(m.worktreeRoot)
	if err != nil {
		return err
	}
	if branch == "" || branch == "HEAD" {
		return fmt.Errorf("cannot rebase: detached HEAD")
	}
	target, err := stagemodal.DetectRebaseTarget(m.settings.TicketStore, m.worktreeRoot)
	if err != nil {
		return rebaseTargetError{err}
	}
	openConfirm(
		m,
		fmt.Sprintf("Rebase branch %s on %s?", branch, target),
		nil,
		stagemodal.ConfirmRebase,
		target,
		branch,
	)
	return nil
}

// rebaseTargetError marks a trunk-resolution failure, which the status view
// shows as a warning rather than a git error.
type rebaseTargetError struct{ error }

func (e rebaseTargetError) Unwrap() error { return e.error }

func openAmendConfirm(m *Model) error {
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
	openConfirm(m, "Amend last commit?", lines, stagemodal.ConfirmAmend, "", "")
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
