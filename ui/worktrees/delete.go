package worktrees

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
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

// deleteProgress is the modal that tracks a batch of deletes, at most
// maxConcurrentDeletes in flight at once.
type deleteProgress struct {
	queue    []git.Worktree    // worktrees pending deletion in the current batch
	inFlight int               // number of concurrent deletes in progress
	steps    []components.Step // one per worktree in the batch
	results  []deleteResultMsg // accumulated results from the batch
	// spinnerFrame is the page spinner's current frame; the root refreshes it
	// on every tick because the modal does not own the spinner.
	spinnerFrame string
}

const maxConcurrentDeletes = 3

// enterDeleteProgress opens the progress modal and starts up to
// maxConcurrentDeletes concurrent delete operations.
func (m Model) enterDeleteProgress(worktrees []git.Worktree) (Model, tea.Cmd) {
	d := deleteProgress{
		steps:        make([]components.Step, len(worktrees)),
		spinnerFrame: m.spinner.View(),
	}
	for i, wt := range worktrees {
		d.steps[i] = components.Step{
			ID:           wt.Name,
			TitleBefore:  wt.Name,
			RunningTitle: wt.Name,
			TitleAfter:   wt.Name,
			TitleFailed:  wt.Name,
		}
	}

	var cmds []tea.Cmd
	for i, wt := range worktrees {
		if i < maxConcurrentDeletes {
			d.steps[i].IsRunning = true
			d.inFlight++
			cmds = append(cmds, cmdDelete(m.repo, wt))
		} else {
			d.queue = append(d.queue, wt)
		}
	}
	return m.openModal(d), tea.Batch(append(cmds, m.spinner.Tick)...)
}

// Update swallows keys while the batch runs and leaves everything else to the
// page.
func (d deleteProgress) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	if _, ok := msg.(tea.KeyPressMsg); ok {
		return d, nil, modalResult{}
	}
	return d, nil, modalResult{unhandled: true}
}

func (d deleteProgress) InputFocused() bool { return false }

// View renders the progress modal.
func (d deleteProgress) View(_, _ int) string {
	title := "Deleting Worktrees"
	if len(d.steps) == 1 {
		title = "Deleting Worktree"
	}
	body := components.RenderSteps(d.steps, d.spinnerFrame)
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         title,
		Body:          body,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		TitleInBorder: true,
	})
}

// record stores msg's result and returns the command for the next queued
// delete, if any. done is true once nothing is queued or in flight.
func (d deleteProgress) record(repo git.Repo, msg deleteResultMsg) (next deleteProgress, cmds []tea.Cmd, done bool) {
	for i, step := range d.steps {
		if step.ID == msg.name {
			d.steps[i].IsRunning = false
			if msg.err != nil {
				d.steps[i].HasFailed = true
			} else {
				d.steps[i].IsDone = true
			}
			break
		}
	}
	d.results = append(d.results, msg)
	d.inFlight--

	if len(d.queue) > 0 {
		next := d.queue[0]
		d.queue = d.queue[1:]
		for i, step := range d.steps {
			if step.ID == next.Name {
				d.steps[i].IsRunning = true
				break
			}
		}
		d.inFlight++
		cmds = append(cmds, cmdDelete(repo, next))
	}
	return d, cmds, d.inFlight == 0 && len(d.queue) == 0
}

// handleDeleteProgressResult updates progress state and dispatches the next
// queued delete. When all deletes finish it calls completeBatchDelete.
func (m Model) handleDeleteProgressResult(d deleteProgress, msg deleteResultMsg) (tea.Model, tea.Cmd) {
	d, cmds, done := d.record(m.repo, msg)
	m.modal = d
	if done {
		completedM, completionCmd := m.completeBatchDelete(d.results)
		return completedM, tea.Batch(append(cmds, completionCmd)...)
	}
	return m, tea.Batch(cmds...)
}

// completeBatchDelete finalises the batch: shows a notification for single
// deletes and a summary log for multi-deletes, then refreshes the worktree list.
func (m Model) completeBatchDelete(results []deleteResultMsg) (Model, tea.Cmd) {
	m.selectedWorktrees = make(map[string]bool)
	m.modal = nil

	if len(results) == 1 {
		result := results[0]
		if result.err != nil {
			m.lastJobLog = result.err.Error()
			m.lastJobLabel = "Delete failed"
			return m.enterLogsMode(), cmdLoadWorktrees(m.repo, m.settings.TicketStore)
		}
		return m, tea.Batch(notify.Info(fmt.Sprintf("Deleted %s", result.name)), cmdLoadWorktrees(m.repo, m.settings.TicketStore))
	}

	// Multi-delete: build a summary log.
	var successes, failures []string
	for _, r := range results {
		if r.err != nil {
			failures = append(failures, r.name)
		} else {
			successes = append(successes, r.name)
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
