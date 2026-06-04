package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/nav"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var notifyCmd tea.Cmd
	m.notify, notifyCmd = m.notify.Update(msg)

	if nav.IsRepoMutated(msg) {
		m.gate.Mutated()
		// Trust-the-self-reload invariant: the page that emitted RepoMutated
		// self-reloads; stamp it fresh so only the other tabs become stale.
		m.gate.MarkLoaded(m.navState.ActiveTab())
		return m, notifyCmd
	}

	if vs, ok := nav.IsSwitch(msg); ok {
		prev := m.navState.Active()
		tabVS := m.navState.Switch(vs)
		next, cmd := m.applySwitch(tabVS, prev)
		return next, tea.Batch(notifyCmd, cmd)
	}
	if vs, ok := nav.IsOpen(msg); ok {
		return m.handleOpen(vs, notifyCmd)
	}
	if vs, ok := nav.IsViewStateChanged(msg); ok {
		m.navState.ApplyViewStateChanged(vs)
		return m, notifyCmd
	}
	if nav.IsBack(msg) {
		return m.handleBack(notifyCmd)
	}

	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		msg = m.childWindowSizeMsg()
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		type inputFocuser interface{ InputFocused() bool }
		active := m.activePage().model
		if f, ok := active.(inputFocuser); !ok || !f.InputFocused() {
			if next, cmd, handled := m.handleShellChordKey(key); handled {
				return next, tea.Batch(notifyCmd, cmd)
			}
		}
	}
	current := m.activePage()
	prevViewState, prevOK := viewStateOf(current.model)
	nextModel, cmd := current.model.Update(msg)
	current.model = nextModel
	m.setActivePage(current)
	nextViewState, nextOK := viewStateOf(nextModel)
	cmd = nav.AppendViewStateChanged(cmd, m.settings.EnableNavigation, prevViewState, prevOK, nextViewState, nextOK)
	return m, tea.Batch(notifyCmd, cmd)
}

func (m Model) handleOpen(vs nav.ViewState, notifyCmd tea.Cmd) (Model, tea.Cmd) {
	tabVS := m.navState.Open(vs)
	var outgoing tea.Model
	if len(m.history) > 0 {
		outgoing = m.history[len(m.history)-1].model
	} else {
		outgoing = m.livePageByTab[m.navState.LiveTab()].model
	}
	entry := m.newHistoryEntry(tabVS)
	m.history = append(m.history, entry)
	return m, tea.Batch(notifyCmd, tea.ClearScreen, onPageDeactivatedCmd(outgoing), entry.model.Init(), m.resizeCurrentCmd())
}

func (m Model) handleBack(notifyCmd tea.Cmd) (Model, tea.Cmd) {
	_, quit := m.navState.Back()
	if quit {
		return m, tea.Batch(notifyCmd, tea.Quit)
	}
	popped := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.restoreLogSelectionFromPoppedPage(popped)
	return m, tea.Batch(notifyCmd, tea.ClearScreen, onPageDeactivatedCmd(popped.model), onPageActivatedCmd(m.activePage().model), m.resizeCurrentCmd())

}

func viewStateOf(model tea.Model) (nav.ViewState, bool) {
	if vsp, ok := model.(nav.ViewStateProvider); ok {
		return vsp.CurrentViewState()
	}
	return nav.ViewState{}, false
}
