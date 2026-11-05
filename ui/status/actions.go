package status

import (
	"github.com/elentok/gx/ui/status/stagemodal"
)

func recordCommandOutput(m *Model, title, out string) {
	m.git.SetOutput(title, out)
}

func openOutputModal(m *Model) {
	m.git.OpenOutput(m.width, m.height)
	m.keys.Reset()
}

func openConfirm(m *Model, title string, lines []string, action stagemodal.ConfirmAction, remote, branch string) {
	openConfirmRequest(m, stagemodal.ConfirmRequest{Title: title, Lines: lines, Action: action, Remote: remote, Branch: branch})
}

func openConfirmRequest(m *Model, req stagemodal.ConfirmRequest) {
	m.keys.Reset()
	m.modals.OpenConfirm(req)
}

func openRunning(m *Model, title string, runner *stagemodal.Runner) {
	m.keys.Reset()
	m.modals.OpenRunning(m.width, m.height, title, runner)
	runner.Start()
}
