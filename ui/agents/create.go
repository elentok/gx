package agents

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/agents/create"
	"github.com/elentok/gx/ui/agents/jump"
)

// WithCwdProject names the project of the repo gx was opened in.
func (m Model) WithCwdProject(name string) Model {
	m.cwdProject = name
	return m
}

// openCreate opens the form on the project in focus: the selected session's,
// else the cwd repo's; with neither the form falls back to the first project.
func (m Model) openCreate() (Model, tea.Cmd) {
	p := create.Prefill{Project: m.cwdProject}
	if m.cursor < len(m.sessions) {
		p.Project = m.sessions[m.cursor].Project
	}
	return m.OpenCreateForm(p)
}

// OpenCreateForm opens the create form pre-filled with p; without a client
// there is no server to create on.
func (m Model) OpenCreateForm(p create.Prefill) (Model, tea.Cmd) {
	if m.actions.client == nil {
		return m, nil
	}
	m.create = create.New(m.settings.Home(), p)
	return m, m.create.Load(m.listProjects)
}

func (m Model) jumpCtx() jump.Ctx {
	return jump.Ctx{Sessions: m.sessions, Now: m.now, Width: m.width, Height: m.height, Settings: m.settings, SpinnerView: m.spinner.View()}
}

func (m Model) createCtx() create.Ctx {
	return create.Ctx{Height: m.height, MainWidth: m.mainWidth(), Sessions: m.actions.sessionAPI}
}

// selectPending refreshes, first selecting and focusing the just-created
// session when it is listed.
func (m Model) selectPending() (tea.Model, tea.Cmd) {
	i := slices.IndexFunc(m.sessions, func(s wire.SessionInfo) bool { return s.ID == m.pendingSelect })
	if m.pendingSelect == "" || i < 0 {
		return m.refresh()
	}
	m.pendingSelect = ""
	return m.selectAndFocus(i)
}
