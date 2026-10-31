package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// harnessPicker is the Queue tab's "Default harness..." modal. project is ""
// for the global value.
type harnessPicker struct {
	open    bool
	project string
	menu    components.MenuState
}

// defaultHarnessReadMsg carries the server's current Default harness.
type defaultHarnessReadMsg struct {
	status server.DefaultHarnessStatus
	err    error
}

// defaultHarnessWroteMsg carries the server's answer to a write.
type defaultHarnessWroteMsg struct {
	res server.SetDefaultHarnessResult
	err error
}

func (m QueueModel) handleDefaultHarnessKey() (tea.Model, tea.Cmd) {
	if m.serverAPI == nil || m.serverDown() {
		return m, notify.Warning(serverNotRunning)
	}
	api := m.serverAPI
	return m, func() tea.Msg {
		st, err := api.DefaultHarness(context.Background())
		return defaultHarnessReadMsg{status: st, err: err}
	}
}

// openHarnessPicker builds the picker for the project filter, with the cursor
// on the value in effect for that scope.
func (m QueueModel) openHarnessPicker(st server.DefaultHarnessStatus) QueueModel {
	items := []components.MenuItem{
		{Label: "Claude", Value: "claude"},
		{Label: "Codex", Value: "codex"},
	}
	current := st.Default
	if m.projectFilter != "" {
		items = append([]components.MenuItem{{Label: "Follow global", Value: ""}}, items...)
		current = ""
		for _, p := range st.Projects {
			if p.Project == m.projectFilter {
				current = p.Harness
			}
		}
	}
	cursor := 0
	for i, it := range items {
		if it.Value == current {
			cursor = i
		}
	}
	m.harnessPicker = harnessPicker{
		open:    true,
		project: m.projectFilter,
		menu:    components.MenuState{Items: items, Cursor: cursor},
	}
	return m
}

func (m QueueModel) updateHarnessPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	next, decided, accepted, handled := components.UpdateMenu(msg, m.harnessPicker.menu)
	if !handled {
		return m, nil
	}
	m.harnessPicker.menu = next
	if !decided {
		return m, nil
	}
	m.harnessPicker.open = false
	item, ok := next.Selected()
	if !accepted || !ok {
		return m, nil
	}
	req := server.SetDefaultHarnessRequest{Scope: "global", Harness: item.Value}
	if m.harnessPicker.project != "" {
		req = server.SetDefaultHarnessRequest{Scope: "project", Project: m.harnessPicker.project, Harness: item.Value}
	}
	api := m.serverAPI
	return m, func() tea.Msg {
		res, err := api.SetDefaultHarness(context.Background(), req)
		return defaultHarnessWroteMsg{res: res, err: err}
	}
}

// updateDefaultHarness handles the picker's read and write results; ok is false
// for any other msg.
func (m QueueModel) updateDefaultHarness(msg tea.Msg) (QueueModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case defaultHarnessReadMsg:
		if msg.err != nil {
			return m, harnessErrorToast("default harness: ", msg.err), true
		}
		return m.openHarnessPicker(msg.status), nil, true
	case defaultHarnessWroteMsg:
		switch {
		case msg.err != nil:
			return m, harnessErrorToast("", msg.err), true
		case msg.res.Refused:
			return m, notify.Error("refused: " + msg.res.Message), true
		case msg.res.Notice != "":
			return m, notify.Success(msg.res.Notice), true
		}
		return m, notify.Success("default harness unchanged"), true
	}
	return m, nil, false
}

func harnessErrorToast(prefix string, err error) tea.Cmd {
	if apiclient.IsNotRunning(err) {
		return notify.Warning(serverNotRunning)
	}
	return notify.Error(prefix + err.Error())
}

func (m QueueModel) harnessPickerView() string {
	title, prompt := "Default harness", "All projects"
	if p := m.harnessPicker.project; p != "" {
		title, prompt = p, "Default harness"
	}
	return components.RenderMenuModal(
		title, ui.StyleDim.Render(prompt), m.harnessPicker.menu, "",
		ui.ColorBlue, ui.ColorBlue, ui.ColorSubtle, ui.ColorGreen,
		max(40, m.width/3),
	)
}
