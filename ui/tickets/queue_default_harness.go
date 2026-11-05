package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
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
	status wire.DefaultHarnessStatus
	err    error
}

// defaultHarnessWroteMsg carries the server's answer to a write.
type defaultHarnessWroteMsg struct {
	res wire.SetDefaultHarnessResult
	err error
}

// readCmd is the "H" key: read the server's Default harness, or warn when the
// server cannot be reached.
func (harnessPicker) readCmd(server queueServer, admin tabapi.AdminAPI) tea.Cmd {
	if server.serverAPI == nil || admin == nil || server.down() {
		return notify.Warning(rowactions.ServerNotRunning)
	}
	return func() tea.Msg {
		st, err := admin.DefaultHarness(context.Background())
		return defaultHarnessReadMsg{status: st, err: err}
	}
}

// opened builds the picker for the project filter, with the cursor on the value
// in effect for that scope.
func (harnessPicker) opened(st wire.DefaultHarnessStatus, projectFilter string) harnessPicker {
	items := []components.MenuItem{
		{Label: "Claude", Value: "claude"},
		{Label: "Codex", Value: "codex"},
	}
	current := st.Default
	if projectFilter != "" {
		items = append([]components.MenuItem{{Label: "Follow global", Value: ""}}, items...)
		current = ""
		for _, p := range st.Projects {
			if p.Project == projectFilter {
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
	return harnessPicker{
		open:    true,
		project: projectFilter,
		menu:    components.MenuState{Items: items, Cursor: cursor},
	}
}

func (p harnessPicker) updateKey(msg tea.KeyPressMsg, admin tabapi.AdminAPI) (harnessPicker, tea.Cmd) {
	next, decided, accepted, handled := components.UpdateMenu(msg, p.menu)
	if !handled {
		return p, nil
	}
	p.menu = next
	if !decided {
		return p, nil
	}
	p.open = false
	item, ok := next.Selected()
	if !accepted || !ok {
		return p, nil
	}
	req := wire.SetDefaultHarnessRequest{Scope: "global", Harness: item.Value}
	if p.project != "" {
		req = wire.SetDefaultHarnessRequest{Scope: "project", Project: p.project, Harness: item.Value}
	}
	return p, func() tea.Msg {
		res, err := admin.SetDefaultHarness(context.Background(), req)
		return defaultHarnessWroteMsg{res: res, err: err}
	}
}

// update handles the picker's read and write results; ok is false for any other
// msg.
func (p harnessPicker) update(msg tea.Msg, projectFilter string) (harnessPicker, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case defaultHarnessReadMsg:
		if msg.err != nil {
			return p, harnessErrorToast("default harness: ", msg.err), true
		}
		return p.opened(msg.status, projectFilter), nil, true
	case defaultHarnessWroteMsg:
		switch {
		case msg.err != nil:
			return p, harnessErrorToast("", msg.err), true
		case msg.res.Refused:
			return p, notify.Error("refused: " + msg.res.Message), true
		case msg.res.Notice != "":
			return p, notify.Success(msg.res.Notice), true
		}
		return p, notify.Success("default harness unchanged"), true
	}
	return p, nil, false
}

func harnessErrorToast(prefix string, err error) tea.Cmd {
	if apiclient.IsNotRunning(err) {
		return notify.Warning(rowactions.ServerNotRunning)
	}
	return notify.Error(prefix + err.Error())
}

func (p harnessPicker) view(width int) string {
	title, prompt := "Default harness", "All projects"
	if p.project != "" {
		title, prompt = p.project, "Default harness"
	}
	return components.RenderMenuModal(
		title, ui.StyleDim.Render(prompt), p.menu, "",
		ui.ColorBlue, ui.ColorBlue, ui.ColorSubtle, ui.ColorGreen,
		max(40, width/3),
	)
}
