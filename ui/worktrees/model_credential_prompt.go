package worktrees

import (
	"fmt"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func newCredentialInput(secret bool) textinput.Model {
	ti := textinput.New()
	ti.Focus()
	if secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}
	return ti
}

// credentialModal asks the user for a credential a running job is waiting on.
type credentialModal struct {
	input  textinput.Model
	text   string
	runner *components.CommandRunner
}

func (m Model) enterCredentialPrompt(prompt components.CredentialPrompt) Model {
	return m.openModal(credentialModal{
		input:  newCredentialInput(prompt.Kind == components.PromptKindSecret),
		text:   prompt.Text,
		runner: m.jobs.runner,
	})
}

// credentialOpen reports whether the credential modal is the open modal.
func (m Model) credentialOpen() bool {
	_, ok := m.modal.(credentialModal)
	return ok
}

func (c credentialModal) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			if c.runner != nil {
				c.runner.Cancel()
			}
			return c, nil, modalResult{close: true}
		case "enter":
			if c.runner != nil {
				_ = c.runner.SubmitPromptInput(c.input.Value())
			}
			return c, nil, modalResult{close: true}
		}
	case tea.PasteMsg:
	default:
		return c, nil, modalResult{unhandled: true}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd, modalResult{}
}

func (c credentialModal) InputFocused() bool { return true }

func (c credentialModal) View(_, _ int) string {
	input := c.input.View()
	if input == "" {
		input = " "
	}
	return components.RenderInputModal(
		"Credential Required",
		c.text,
		input,
		ui.HintSubmitCancel(),
		ui.ColorBorder,
		ui.ColorGreen,
		ui.ColorGray,
		0,
	)
}

func (m Model) finishPromptableJob(err error) (tea.Model, tea.Cmd) {
	done, ok := m.jobs.finish(m.repo)
	if !ok {
		m.spinnerActive = false
		m.modal = nil
		m.mode = modeNormal
		return m, nil
	}

	wt, kind, output, log := done.wt, done.kind, done.output, done.log
	m.modal = nil
	m.mode = modeNormal
	m.spinnerActive = false
	m.lastJobLog = log
	m.lastJobLabel = promptableJobOutputTitle(kind)

	switch kind {
	case promptableJobPushFetch:
		if err != nil {
			return m.showError(err.Error()), nil
		}
		div, divErr := git.DetectPushDivergenceAfterFetch(wt.Path, wt.Branch)
		if divErr != nil {
			return m.showError(divErr.Error()), nil
		}
		if div != nil {
			return m.enterPushDivergedMode(wt, div), nil
		}
		return m, cmdStartPromptableJob(promptableJobPush, wt, log, false)
	case promptableJobPush:
		if err != nil {
			if git.IsNonFastForwardPushError(err) {
				return m.enterConfirm(forcePushPrompt(wt), cmdStartPromptableJob(promptableJobForcePush, wt, log, false), "Force-pushing "+wt.Name+"…"), nil
			}
			return m.showError(err.Error()), nil
		}
		cmds := []tea.Cmd{notify.Info(ui.MessageComplete("push"))}
		if wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadPreviewData(m.repo, wt))
		}
		prURL := git.ExtractPRURL(output)
		if prURL != "" {
			prompt := fmt.Sprintf("Open pull request page?\n\n%s", prURL)
			m = m.enterConfirmDefaultYes(prompt, cmdOpenURL(prURL), "")
			return m, tea.Batch(cmds...)
		}
		return m, tea.Batch(cmds...)
	case promptableJobForcePush:
		if err != nil {
			return m.showError(err.Error()), nil
		}
		cmds := []tea.Cmd{notify.Info(ui.MessageComplete("force push"))}
		if wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadPreviewData(m.repo, wt))
		}
		return m, tea.Batch(cmds...)
	default:
		return m, nil
	}
}
