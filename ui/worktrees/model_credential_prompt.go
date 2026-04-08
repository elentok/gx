package worktrees

import (
	"fmt"

	"gx/git"
	"gx/ui"
	"gx/ui/components"

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

func (m Model) enterCredentialPrompt(prompt components.CredentialPrompt) Model {
	m.mode = modeCredentialPrompt
	m.textInput = newCredentialInput(prompt.Kind == components.PromptKindSecret)
	m.confirmPrompt = prompt.Text
	return m
}

func (m Model) handleCredentialKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.jobRunner != nil {
			m.jobRunner.Cancel()
		}
		m.mode = modeNormal
		return m, nil
	case "enter":
		if m.jobRunner != nil {
			_ = m.jobRunner.SubmitPromptInput(m.textInput.Value())
		}
		m.mode = modeNormal
		return m, nil
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) credentialModalView() string {
	input := m.textInput.View()
	if input == "" {
		input = " "
	}
	return components.RenderInputModal(
		"Credential Required",
		m.confirmPrompt,
		input,
		"enter submit · esc cancel",
		ui.ColorBorder,
		ui.ColorGreen,
		ui.ColorGray,
		0,
	)
}

func (m Model) finishPromptableJob(err error) (tea.Model, tea.Cmd) {
	if m.jobRunner == nil || m.jobWorktree == nil || m.jobLog == nil {
		m.spinnerActive = false
		m.mode = modeNormal
		return m, nil
	}

	wt := *m.jobWorktree
	args := promptableJobArgs(m.repo, m.jobKind, wt)
	output := m.jobRunner.Output()
	m.jobLog.AppendCommand("git", args, output)
	log := m.jobLog.String()
	kind := m.jobKind
	stashed := m.jobStashed

	m.jobRunner = nil
	m.jobWorktree = nil
	m.jobLog = nil
	m.jobStashed = false
	m.mode = modeNormal
	m.spinnerActive = false
	m.lastJobLog = log
	m.lastJobLabel = promptableJobOutputTitle(kind)

	switch kind {
	case promptableJobPull:
		if err != nil {
			if stashed {
				prompt := fmt.Sprintf("Pull failed: %s\n\nPop stash?", err.Error())
				return m.enterConfirm(prompt, cmdStashPop(wt.Path, "pull", log), "Popping stash…"), nil
			}
			return m.showError(err.Error()), nil
		}
		if stashed {
			m.spinnerActive = true
			m.spinnerLabel = "Popping stash…"
			return m, tea.Batch(cmdStashPop(wt.Path, "pull", log), m.spinner.Tick)
		}
		m.statusGen++
		m.statusMsg = "Pulled"
		if log != "" {
			m.statusMsg += "  ·  o  view output"
		}
		cmds := []tea.Cmd{cmdClearStatus(m.statusGen)}
		if wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadSidebarData(m.repo, wt))
			if wt.Branch == m.repo.MainBranch {
				for _, w := range m.worktrees {
					if w.Branch != "" {
						cmds = append(cmds, cmdLoadBaseStatus(m.repo, w.Branch))
					}
				}
			}
		}
		return m, tea.Batch(cmds...)
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
		m.statusGen++
		m.statusMsg = "Pushed"
		if log != "" {
			m.statusMsg += "  ·  o  view output"
		}
		cmds := []tea.Cmd{cmdClearStatus(m.statusGen)}
		if wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadSidebarData(m.repo, wt))
		}
		prURL := git.ExtractPRURL(output)
		if prURL != "" {
			prompt := fmt.Sprintf("Open pull request page?\n\n%s", prURL)
			m = m.enterConfirm(prompt, cmdOpenURL(prURL), "")
			m.confirmYes = true
			return m, tea.Batch(cmds...)
		}
		return m, tea.Batch(cmds...)
	case promptableJobForcePush:
		if err != nil {
			return m.showError(err.Error()), nil
		}
		m.statusGen++
		m.statusMsg = "Force-pushed"
		if log != "" {
			m.statusMsg += "  ·  o  view output"
		}
		cmds := []tea.Cmd{cmdClearStatus(m.statusGen)}
		if wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadSidebarData(m.repo, wt))
		}
		return m, tea.Batch(cmds...)
	default:
		return m, nil
	}
}
