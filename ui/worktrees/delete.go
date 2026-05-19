package worktrees

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

// deleteResultMsg is sent when a single delete operation completes.
type deleteResultMsg struct {
	name string
	err  error
}

// startBatchDeleteMsg triggers enterDeleteProgress with the given batch.
type startBatchDeleteMsg struct {
	worktrees []git.Worktree
}

func cmdStartBatchDelete(worktrees []git.Worktree) tea.Cmd {
	return func() tea.Msg {
		return startBatchDeleteMsg{worktrees: worktrees}
	}
}

// cmdDelete removes the worktree directory and force-deletes its branch.
// name is always populated in the returned msg so callers can match results.
func cmdDelete(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		if err := git.RemoveWorktree(repo, wt.Name, true); err != nil {
			return deleteResultMsg{name: wt.Name, err: err}
		}
		if wt.Branch != "" {
			if err := git.DeleteLocalBranch(repo, wt.Branch, true); err != nil {
				return deleteResultMsg{name: wt.Name, err: err}
			}
		}
		return deleteResultMsg{name: wt.Name}
	}
}

// enterDeleteConfirm shows a confirmation modal for the selected worktrees
// (or the cursor worktree if none are tagged).
func (m Model) enterDeleteConfirm() (tea.Model, tea.Cmd) {
	var batch []git.Worktree
	if len(m.selectedWorktrees) > 0 {
		for i, wt := range m.worktrees {
			if m.selectedWorktrees[wt.Name] {
				batch = append(batch, m.worktrees[i])
			}
		}
	} else {
		wt := m.cursorWorktree()
		if wt != nil {
			batch = []git.Worktree{*wt}
		}
	}
	if len(batch) == 0 {
		return m, nil
	}

	var prompt string
	if len(batch) == 1 {
		prompt = fmt.Sprintf("Delete worktree '%s'", batch[0].Name)
		if batch[0].Branch != "" {
			prompt += fmt.Sprintf(" (branch: %s)", batch[0].Branch)
		}
		prompt += "?"
	} else {
		prompt = fmt.Sprintf("Delete %d worktrees?", len(batch))
	}

	m = m.enterConfirm(prompt, cmdStartBatchDelete(batch), "")
	if len(batch) > 1 {
		names := make([]string, len(batch))
		for i, wt := range batch {
			names[i] = wt.Name
		}
		m.confirmItems = names
	}
	return m, nil
}

// enterDeleteProgress switches to modeDeleteProgress and starts up to 3
// concurrent delete operations.
func (m Model) enterDeleteProgress(worktrees []git.Worktree) (Model, tea.Cmd) {
	m.mode = modeDeleteProgress
	m.deleteQueue = nil
	m.deleteResults = nil
	m.deleteInFlight = 0

	m.deleteSteps = make([]components.Step, len(worktrees))
	for i, wt := range worktrees {
		m.deleteSteps[i] = components.Step{
			ID:           wt.Name,
			TitleBefore:  wt.Name,
			RunningTitle: wt.Name,
			TitleAfter:   wt.Name,
			TitleFailed:  wt.Name,
		}
	}

	var cmds []tea.Cmd
	for i, wt := range worktrees {
		if i < 3 {
			m.deleteSteps[i].IsRunning = true
			m.deleteInFlight++
			cmds = append(cmds, cmdDelete(m.repo, wt))
		} else {
			m.deleteQueue = append(m.deleteQueue, wt)
		}
	}
	return m, tea.Batch(append(cmds, m.spinner.Tick)...)
}

// handleDeleteProgressResult updates progress state and dispatches the next
// queued delete. When all deletes finish it calls completeBatchDelete.
func (m Model) handleDeleteProgressResult(msg deleteResultMsg) (tea.Model, tea.Cmd) {
	for i, step := range m.deleteSteps {
		if step.ID == msg.name {
			m.deleteSteps[i].IsRunning = false
			if msg.err != nil {
				m.deleteSteps[i].HasFailed = true
			} else {
				m.deleteSteps[i].IsDone = true
			}
			break
		}
	}
	m.deleteResults = append(m.deleteResults, msg)
	m.deleteInFlight--

	var cmds []tea.Cmd
	if len(m.deleteQueue) > 0 {
		next := m.deleteQueue[0]
		m.deleteQueue = m.deleteQueue[1:]
		for i, step := range m.deleteSteps {
			if step.ID == next.Name {
				m.deleteSteps[i].IsRunning = true
				break
			}
		}
		m.deleteInFlight++
		cmds = append(cmds, cmdDelete(m.repo, next))
	}

	if m.deleteInFlight == 0 && len(m.deleteQueue) == 0 {
		completedM, completionCmd := m.completeBatchDelete()
		return completedM, tea.Batch(append(cmds, completionCmd)...)
	}
	return m, tea.Batch(cmds...)
}

// completeBatchDelete finalises the batch: shows a notification for single
// deletes and a summary log for multi-deletes, then refreshes the worktree list.
func (m Model) completeBatchDelete() (Model, tea.Cmd) {
	m.selectedWorktrees = make(map[string]bool)
	m.mode = modeNormal

	if len(m.deleteResults) == 1 {
		result := m.deleteResults[0]
		if result.err != nil {
			m.lastJobLog = result.err.Error()
			m.lastJobLabel = "Delete failed"
			return m.enterLogsMode(), cmdLoadWorktrees(m.repo)
		}
		return m, tea.Batch(notify.Info(fmt.Sprintf("Deleted %s", result.name)), cmdLoadWorktrees(m.repo))
	}

	// Multi-delete: build a summary log.
	var successes, failures []string
	for _, r := range m.deleteResults {
		if r.err != nil {
			failures = append(failures, r.name)
		} else {
			successes = append(successes, r.name)
		}
	}
	total := len(m.deleteResults)

	var sb strings.Builder
	if len(failures) > 0 {
		sb.WriteString(fmt.Sprintf("Failed to delete %d/%d:\n", len(failures), total))
		for _, name := range failures {
			sb.WriteString("  • " + name + "\n")
		}
	}
	if len(successes) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("Successfully deleted %d/%d:\n", len(successes), total))
		for _, name := range successes {
			sb.WriteString("  • " + name + "\n")
		}
	}

	m.lastJobLog = strings.TrimRight(sb.String(), "\n")
	m.lastJobLabel = "Delete summary"
	return m.enterLogsMode(), cmdLoadWorktrees(m.repo)
}

// deleteProgressModalView renders the progress modal for modeDeleteProgress.
func (m Model) deleteProgressModalView() string {
	title := "Deleting Worktrees"
	if len(m.deleteSteps) == 1 {
		title = "Deleting Worktree"
	}
	body := components.RenderSteps(m.deleteSteps, m.spinner.View())
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         title,
		Body:          body,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		TitleInBorder: true,
	})
}
