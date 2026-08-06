package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// notifyCmd carries every overlay's cmd (not just notify.Model's) through
	// each return point below, since only one variable needs threading.
	var notifyCmd tea.Cmd
	m.notify, notifyCmd = m.notify.Update(msg)
	// Captured here, once, so no page/tab needs its own notifylog wiring.
	switch v := msg.(type) {
	case notify.NotifyMsg:
		m.notifyLog.Append(v)
	case notify.CloseMsg:
		m.notifyLog.Close(v.ID)
	}
	var loopStatusCmd tea.Cmd
	m.loopStatus, loopStatusCmd = m.loopStatus.Update(msg)
	notifyCmd = tea.Batch(notifyCmd, loopStatusCmd)

	if m.quitConfirm.IsOpen {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			next, cmd, _ := m.quitConfirm.Update(key)
			m.quitConfirm = next
			return m, tea.Batch(notifyCmd, cmd)
		}
		if click, ok := msg.(tea.MouseClickMsg); ok {
			next, cmd, _ := m.quitConfirm.UpdateMouse(click, m.width, m.width, m.height)
			m.quitConfirm = next
			return m, tea.Batch(notifyCmd, cmd)
		}
		return m, notifyCmd
	}

	if m.notifyHistory.IsOpen {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			next, cmd, _ := m.notifyHistory.Update(key)
			m.notifyHistory = next
			return m, tea.Batch(notifyCmd, cmd)
		}
		if _, ok := msg.(tea.MouseMsg); ok {
			return m, notifyCmd
		}
		// Everything else (ticks, window-size, ...) falls through to the
		// normal page-update path below, exactly as if the modal were closed,
		// so background polling loops keep rearming themselves.
	}

	if click, ok := msg.(tea.MouseClickMsg); ok {
		mouse := click.Mouse()
		if mouse.Button == tea.MouseLeft {
			if id, hit := m.tabHitAt(mouse.X, mouse.Y); hit {
				next, cmd := m.switchTab(nav.ViewState{Tab: id})
				return next, tea.Batch(notifyCmd, cmd)
			}
		}
	}

	if nav.IsRepoMutated(msg) {
		m.gate.Mutated()
		// Trust-the-self-reload invariant: the page that emitted RepoMutated
		// self-reloads; stamp it fresh so only the other tabs become stale.
		m.gate.MarkLoaded(m.navState.ActiveTab())
		return m, notifyCmd
	}

	if vs, ok := nav.IsSwitch(msg); ok {
		type modalOpener interface{ ModalOpen() bool }
		active := m.activePage().model
		if mo, ok := active.(modalOpener); ok && mo.ModalOpen() {
			return m, tea.Batch(notifyCmd, notify.Info("close the modal first"))
		}
		prev := m.navState.Active()
		tabVS := m.navState.Switch(vs)
		next, cmd := m.applySwitch(tabVS, prev)
		return next, tea.Batch(notifyCmd, cmd)
	}
	if vs, ok := nav.IsOpen(msg); ok {
		return m.handleOpen(vs, notifyCmd)
	}
	if vs, ok := nav.IsViewStateChanged(msg); ok {
		resolved := m.navState.ApplyViewStateChanged(vs)
		// Keep the live page's stamped viewState aligned with the resolved context.
		// The page reports its normalized ref (e.g. "" -> "HEAD") here; without this
		// sync the stamp drifts from navState memory and the next tab switch sees a
		// context mismatch and needlessly rebuilds the page (dropping cached rows).
		if len(m.history) == 0 {
			if live, ok := m.livePageByTab[resolved.Tab]; ok {
				live.viewState = resolved
				m.livePageByTab[resolved.Tab] = live
			}
		}
		return m, notifyCmd
	}
	if nav.IsBack(msg) {
		return m.handleBack(notifyCmd)
	}
	if nav.IsForceQuit(msg) {
		return m.attemptQuit(notifyCmd)
	}

	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		msg = m.childWindowSizeMsg()
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		type inputFocuser interface{ InputFocused() bool }
		type keyManagerProvider interface{ KeyManager() keys.Manager }
		active := m.activePage().model
		var activeHasPendingChord bool
		if kmp, ok := active.(keyManagerProvider); ok {
			activeHasPendingChord = len(kmp.KeyManager().Prefix()) > 0
		}
		if f, ok := active.(inputFocuser); (!ok || !f.InputFocused()) && !activeHasPendingChord {
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
		return m.attemptQuit(notifyCmd)
	}
	popped := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.restoreLogSelectionFromPoppedPage(popped)
	return m, tea.Batch(notifyCmd, tea.ClearScreen, onPageDeactivatedCmd(popped.model), onPageActivatedCmd(m.activePage().model), m.resizeCurrentCmd())

}

// attemptQuit is the single place that decides whether gx actually exits:
// handleBack reaches it after unwinding the nav stack to empty, and
// nav.ForceQuit (ctrl+c) reaches it directly since ctrl+c bypasses the stack
// entirely. See canQuit for the guard it applies.
func (m Model) attemptQuit(notifyCmd tea.Cmd) (Model, tea.Cmd) {
	if !m.canQuit() {
		m.quitConfirm = m.quitConfirm.Open(confirm.Options{
			Prompt:    "A ralph-loop is in progress — closing gx may leave the worktree mid-operation. Quit anyway?",
			AcceptCmd: tea.Quit,
		})
		return m, notifyCmd
	}
	return m, tea.Batch(notifyCmd, tea.Quit)
}

func viewStateOf(model tea.Model) (nav.ViewState, bool) {
	if vsp, ok := model.(nav.ViewStateProvider); ok {
		return vsp.CurrentViewState()
	}
	return nav.ViewState{}, false
}
