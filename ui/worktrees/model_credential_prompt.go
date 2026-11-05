package worktrees

import (
	"fmt"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

func (m Model) finishPromptableJob(err error) (tea.Model, tea.Cmd) {
	done, ok := m.jobs.finish(m.repo)
	if !ok {
		m.spinnerActive = false
		m.modal = nil
		return m, nil
	}

	wt, kind, output, log := done.wt, done.kind, done.output, done.log
	m.modal = nil
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
				return m.enterConfirm(confirm.Options{Prompt: forcePushPrompt(wt), AcceptCmd: cmdStartPromptableJob(promptableJobForcePush, wt, log, false), SpinnerLabel: "Force-pushing " + wt.Name + "…"}), nil
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
			m = m.enterConfirm(confirm.Options{Prompt: prompt, AcceptCmd: cmdOpenURL(prURL), DefaultYes: true})
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
