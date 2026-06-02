package status

import (
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/stash"

	tea "charm.land/bubbletea/v2"
)

// hasStashableChanges reports whether the current file list holds anything that
// the requested stash variant would capture. Untracked files are excluded: the
// stash is run without --include-untracked (out of scope), so `git stash push`
// ignores them and a tree of only-untracked files is "nothing to stash".
func (m Model) hasStashableChanges(stagedOnly bool) bool {
	for _, f := range m.statusData.files {
		if f.HasStagedChanges() {
			return true
		}
		if !stagedOnly && !f.IsUntracked() && f.HasUnstagedChanges() {
			return true
		}
	}
	return false
}

func (m Model) handleStashUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.stash.Update(msg)
	m.stash = next
	if !result.Done {
		return m, cmd
	}
	if result.Err != nil {
		m.output.Set("Stash output", result.Err.Error())
		return m, notify.Error("stash failed: " + result.Err.Error())
	}
	switch result.Outcome {
	case stash.OutcomeStashed:
		label := "stashed all changes"
		if result.StagedOnly {
			label = "stashed staged changes"
		}
		return m, tea.Batch(notify.Success(label), m.refresh())
	default:
		return m, nil
	}
}
