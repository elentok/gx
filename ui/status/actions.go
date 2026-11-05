package status

import (
	"github.com/elentok/gx/ui/status/stagemodal"
)

func (m *Model) recordCommandOutput(title, out string) {
	m.git.SetOutput(title, out)
}

func (m *Model) openOutputModal() {
	m.git.OpenOutput(m.width, m.height)
	m.keys.Reset()
}

func (m *Model) openConfirm(title string, lines []string, action stagemodal.ConfirmAction, remote, branch string) {
	m.openConfirmRequest(stagemodal.ConfirmRequest{Title: title, Lines: lines, Action: action, Remote: remote, Branch: branch})
}

func (m *Model) openConfirmRequest(req stagemodal.ConfirmRequest) {
	m.keys.Reset()
	m.modals.OpenConfirm(req)
}

func (m *Model) openRunning(title string, runner *stagemodal.Runner) {
	m.keys.Reset()
	m.modals.OpenRunning(m.width, m.height, title, runner)
	runner.Start()
}
