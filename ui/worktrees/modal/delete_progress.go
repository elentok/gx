package modal

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	tea "charm.land/bubbletea/v2"
)

// DeleteResultMsg is sent when a single delete operation completes.
type DeleteResultMsg struct {
	Name string
	Err  error
}

// CmdDelete removes the worktree directory and force-deletes its branch.
// Name is always populated in the returned msg so callers can match results.
func CmdDelete(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		if err := git.RemoveWorktree(repo, wt.Name, true); err != nil {
			return DeleteResultMsg{Name: wt.Name, Err: err}
		}
		if wt.Branch != "" {
			if err := git.DeleteLocalBranch(repo, wt.Branch, true); err != nil {
				return DeleteResultMsg{Name: wt.Name, Err: err}
			}
		}
		return DeleteResultMsg{Name: wt.Name}
	}
}

// DeleteProgress is the modal that tracks a batch of deletes, at most
// maxConcurrentDeletes in flight at once.
type DeleteProgress struct {
	queue    []git.Worktree    // worktrees pending deletion in the current batch
	inFlight int               // number of concurrent deletes in progress
	steps    []components.Step // one per worktree in the batch
	results  []DeleteResultMsg // accumulated results from the batch
	// spinnerFrame is the page spinner's current frame; the root refreshes it
	// on every tick because the modal does not own the spinner.
	spinnerFrame string
}

const maxConcurrentDeletes = 3

// NewDeleteProgress opens the progress modal and returns the commands that
// start up to maxConcurrentDeletes concurrent delete operations.
func NewDeleteProgress(repo git.Repo, worktrees []git.Worktree, spinnerFrame string) (DeleteProgress, []tea.Cmd) {
	d := DeleteProgress{
		steps:        make([]components.Step, len(worktrees)),
		spinnerFrame: spinnerFrame,
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
			cmds = append(cmds, CmdDelete(repo, wt))
		} else {
			d.queue = append(d.queue, wt)
		}
	}
	return d, cmds
}

// Results are the accumulated results of the batch so far.
func (d DeleteProgress) Results() []DeleteResultMsg { return d.results }

// SetSpinnerFrame returns the modal drawing frame as the spinner.
func (d DeleteProgress) SetSpinnerFrame(frame string) DeleteProgress {
	d.spinnerFrame = frame
	return d
}

// Update swallows keys while the batch runs and leaves everything else to the
// page.
func (d DeleteProgress) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	if _, ok := msg.(tea.KeyPressMsg); ok {
		return d, nil, Result{}
	}
	return d, nil, Result{Unhandled: true}
}

func (d DeleteProgress) InputFocused() bool { return false }

// View renders the progress modal.
func (d DeleteProgress) View(_, _ int) string {
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

// Record stores msg's result and returns the command for the next queued
// delete, if any. done is true once nothing is queued or in flight.
func (d DeleteProgress) Record(repo git.Repo, msg DeleteResultMsg) (next DeleteProgress, cmds []tea.Cmd, done bool) {
	for i, step := range d.steps {
		if step.ID == msg.Name {
			d.steps[i].IsRunning = false
			if msg.Err != nil {
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
		cmds = append(cmds, CmdDelete(repo, next))
	}
	return d, cmds, d.inFlight == 0 && len(d.queue) == 0
}
