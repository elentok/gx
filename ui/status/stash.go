package status

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type stashFinishedMsg struct {
	stagedOnly bool
	output     string
	err        error
}

func newStashInput() textinput.Model {
	ti := textinput.New()
	ti.Focus()
	return ti
}

// openStash runs the empty-state pre-check against the in-memory file list and
// either opens the stash-name modal or returns a "nothing to stash" notice.
func (m *Model) openStash(stagedOnly bool) tea.Cmd {
	if !m.hasStashableChanges(stagedOnly) {
		if stagedOnly {
			return notify.Info("nothing staged to stash")
		}
		return notify.Info("nothing to stash")
	}
	m.stashStagedOnly = stagedOnly
	m.stashInput = newStashInput()
	m.stashOpen = true
	m.keys.Reset()
	return nil
}

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

func (m *Model) handleStashKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.stashOpen = false
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.stashInput.Value())
		stagedOnly := m.stashStagedOnly
		m.stashOpen = false
		return m, cmdStash(m.worktreeRoot, name, stagedOnly)
	}
	var cmd tea.Cmd
	m.stashInput, cmd = m.stashInput.Update(msg)
	return m, cmd
}

func cmdStash(root, name string, stagedOnly bool) tea.Cmd {
	return func() tea.Msg {
		out, err := git.StashPush(root, name, stagedOnly)
		return stashFinishedMsg{stagedOnly: stagedOnly, output: out, err: err}
	}
}

func (m Model) handleStashFinished(msg stashFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.showGitError(fmt.Errorf("stash failed: %w", msg.err))
		return m, nil
	}
	label := "stashed all changes"
	if msg.stagedOnly {
		label = "stashed staged changes"
	}
	return m, tea.Batch(notify.Success(label), m.refresh())
}

func (m Model) stashModalView() string {
	title := "Stash all changes"
	prompt := "Stash all changes (staged + unstaged). Name (optional):"
	if m.stashStagedOnly {
		title = "Stash staged changes"
		prompt = "Stash staged changes only. Name (optional):"
	}
	input := m.stashInput.View()
	if input == "" {
		input = " "
	}
	return components.RenderInputModal(
		title,
		prompt,
		input,
		ui.HintSubmitCancel(),
		ui.ColorBlue,
		ui.ColorBlue,
		ui.ColorSubtle,
		0,
	)
}
