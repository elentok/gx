package worktrees

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/worktrees/modal"

	tea "charm.land/bubbletea/v2"
)

// startBatchDeleteMsg triggers enterDeleteProgress with the given batch.
type startBatchDeleteMsg struct {
	worktrees []git.Worktree
}

func cmdStartBatchDelete(worktrees []git.Worktree) tea.Cmd {
	return func() tea.Msg {
		return startBatchDeleteMsg{worktrees: worktrees}
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

	var items []string
	if len(batch) > 1 {
		items = make([]string, len(batch))
		for i, wt := range batch {
			items[i] = wt.Name
		}
	}
	m = m.enterConfirm(confirm.Options{Prompt: prompt, Items: items, AcceptCmd: cmdStartBatchDelete(batch)})
	return m, nil
}

// enterDeleteProgress opens the progress modal and starts the first deletes.
func (m Model) enterDeleteProgress(worktrees []git.Worktree) (Model, tea.Cmd) {
	d, cmds := modal.NewDeleteProgress(m.repo, worktrees, m.spinner.View())
	return m.openModal(d), tea.Batch(append(cmds, m.spinner.Tick)...)
}

// handleDeleteProgressResult updates progress state and dispatches the next
// queued delete. When all deletes finish it calls completeBatchDelete.
func (m Model) handleDeleteProgressResult(d modal.DeleteProgress, msg modal.DeleteResultMsg) (tea.Model, tea.Cmd) {
	d, cmds, done := d.Record(m.repo, msg)
	m.modal = d
	if done {
		completedM, completionCmd := m.completeBatchDelete(d.Results())
		return completedM, tea.Batch(append(cmds, completionCmd)...)
	}
	return m, tea.Batch(cmds...)
}

// completeBatchDelete finalises the batch: shows a notification for single
// deletes and a summary log for multi-deletes, then refreshes the worktree list.
func (m Model) completeBatchDelete(results []modal.DeleteResultMsg) (Model, tea.Cmd) {
	m.selectedWorktrees = make(map[string]bool)
	m.modal = nil

	if len(results) == 1 {
		result := results[0]
		if result.Err != nil {
			m.lastJobLog = result.Err.Error()
			m.lastJobLabel = "Delete failed"
			return m.enterLogsMode(), cmdLoadWorktrees(m.repo, m.settings.TicketStore)
		}
		return m, tea.Batch(notify.Info(fmt.Sprintf("Deleted %s", result.Name)), cmdLoadWorktrees(m.repo, m.settings.TicketStore))
	}

	// Multi-delete: build a summary log.
	var successes, failures []string
	for _, r := range results {
		if r.Err != nil {
			failures = append(failures, r.Name)
		} else {
			successes = append(successes, r.Name)
		}
	}
	total := len(results)

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
	return m.enterLogsMode(), cmdLoadWorktrees(m.repo, m.settings.TicketStore)
}
