package agents

import (
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/agents/agentstyle"
	"github.com/elentok/gx/ui/agents/jump"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

// digitTabs is the shell's tab order, so leader 1-8 matches plain 1-8.
var digitTabs = []nav.TabID{
	nav.TabWorktrees, nav.TabLog, nav.TabStatus, nav.TabStash,
	nav.TabPRs, nav.TabTickets, nav.TabQueue, nav.TabAgents,
}

// leaderBindings lists the leader actions for the which-key box and help.
var leaderBindings = []keys.Binding{
	{Seq: []string{"j"}, Title: "jump to session"},
	{Seq: []string{"c"}, Title: "new session"},
	{Seq: []string{"x"}, Title: "stop session"},
	{Seq: []string{"d"}, Title: "remove session"},
	{Seq: []string{"r"}, Title: "restore session"},
	{Seq: []string{"a"}, Title: "run action"},
	{Seq: []string{"["}, Title: "copy mode"},
	{Seq: []string{"n"}, Title: "next session"},
	{Seq: []string{"p"}, Title: "previous session"},
	{Seq: []string{"u"}, Title: "next needing attention"},
	{Seq: []string{"b"}, Title: "toggle sidebar"},
	{Seq: []string{"z"}, Title: "toggle AFK"},
	{Seq: []string{"tab"}, Title: "switch focus"},
	{Seq: []string{"1-8"}, Title: "switch gx tab"},
	{Seq: []string{"o"}, Title: "last active tab"},
	{Seq: []string{"ctrl+o"}, Title: "switch view"},
	{Seq: []string{"q"}, Title: "leave tab"},
	{Seq: []string{"i"}, Title: "unlock Queue/Design input"},
	{Seq: []string{"?"}, Title: "help"},
}

var sidebarBindings = []keys.Binding{
	{Seq: []string{"j"}, Display: "j/↓", Title: "next session"},
	{Seq: []string{"k"}, Display: "k/↑", Title: "previous session"},
	{Seq: []string{"enter"}, Title: "focus terminal"},
	{Seq: []string{"u"}, Title: "next needing attention"},
	{Seq: []string{"/"}, Title: "jump to session"},
	{Seq: []string{"c"}, Title: "new session"},
	{Seq: []string{"x"}, Title: "stop session"},
	{Seq: []string{"d"}, Title: "remove session"},
	{Seq: []string{"r"}, Title: "restore session"},
	{Seq: []string{"a"}, Title: "run action"},
	{Seq: []string{"m"}, Title: "session menu"},
	{Seq: []string{"b"}, Title: "toggle sidebar"},
	{Seq: []string{"t", "q"}, Display: "t q", Title: "show/hide Queue agents"},
	{Seq: []string{"t", "d"}, Display: "t d", Title: "show/hide Design agents"},
	{Seq: []string{"?"}, Title: "help"},
}

func leaderKeys(s ui.Settings) []string {
	if len(s.Agents.LeaderKey) == 0 {
		return []string{config.DefaultLeaderKey}
	}
	return s.Agents.LeaderKey
}

func focusSidebarKeys(s ui.Settings) []string {
	if len(s.Agents.FocusSidebarKey) == 0 {
		return []string{config.DefaultFocusSidebarKey}
	}
	return s.Agents.FocusSidebarKey
}

// leaderBindingsFor is leaderBindings with the focus-sidebar key named on the
// switch-focus row, its one-key alternative.
func leaderBindingsFor(s ui.Settings) []keys.Binding {
	bs := slices.Clone(leaderBindings)
	for i, b := range bs {
		if b.Keys() == "tab" {
			bs[i].Title += " (" + focusSidebarKeys(s)[0] + " to sidebar)"
		}
	}
	return bs
}

// updateKey matches the leader before anything else, in either focus, so the
// leader never reaches the session on its own.
func (m Model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	isLeader := slices.Contains(leaderKeys(m.settings), msg.String())
	switch {
	case m.help.IsOpen:
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	case m.jump != nil:
		var res jump.Result
		m.jump, res = m.jump.Update(msg, m.jumpCtx())
		if res.Picked {
			return m.selectAndFocus(res.Session)
		}
		return m.startSpinner()
	case m.create != nil:
		var cmd tea.Cmd
		m.create, cmd, _ = m.create.Update(msg, m.createCtx())
		return m, cmd
	case m.actions.confirm.IsOpen:
		cmd := m.actions.updateConfirmKey(msg)
		return m, cmd
	case m.actions.menu.IsOpen:
		cmd := m.actions.updateMenuKey(msg)
		return m, cmd
	case m.copy != nil:
		// Copy mode takes every key, the leader too, so `g g` stays its own.
		return m.updateCopyKey(msg)
	case m.leaderPending:
		m.leaderPending = false
		if isLeader || slices.Contains(focusSidebarKeys(m.settings), msg.String()) {
			m.attach.SendKey(msg)
			return m, nil
		}
		return m.runLeaderAction(msg.String())
	case isLeader:
		m.leaderPending = true
		return m, nil
	case m.terminalFocused && slices.Contains(focusSidebarKeys(m.settings), msg.String()):
		m.terminalFocused, m.hideSidebar = false, false
		return m, nil
	case m.terminalFocused:
		if a := m.attach; a.Live() && a.ReadOnly {
			return m, notify.Info("read-only: " + leaderKeys(m.settings)[0] + " then i to unlock")
		}
		m.attach.SendKey(msg)
		return m, nil
	}
	return m.updateSidebarKey(msg)
}

// openHelp rebuilds the help, since the leader and focus-sidebar keys are
// configurable, and opens it.
func (m Model) openHelp() Model {
	m.help = help.NewModel([]help.KeySection{
		help.NewKeySection("Terminal", keys.Binding{
			Seq: focusSidebarKeys(m.settings)[:1], Display: strings.Join(focusSidebarKeys(m.settings), "/"), Title: "focus sidebar",
		}),
		help.NewKeySection("Leader ("+leaderKeys(m.settings)[0]+" then)", leaderBindingsFor(m.settings)...),
		help.NewKeySection("Sidebar", sidebarBindings...),
	}, m.settings.About)
	m.help.Open(m.width, m.height)
	return m
}

// runLeaderAction runs the key pressed after the leader; esc and unbound keys
// are dropped.
func (m Model) runLeaderAction(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "j":
		m.jump = &jump.Popup{}
		return m.startSpinner()
	case "c":
		return m.openCreate()
	case "x", "d", "r", "i", "a":
		pick := k
		if k == "i" {
			pick = "unlock"
		}
		s, ok := m.selected()
		cmd := m.actions.pick(pick, s, ok, m.attach)
		return m, cmd
	case "[":
		return m.enterCopyMode()
	case "n":
		m.cursor = m.sidebar.stepCursor(m.sidebarInput(), 1)
	case "p":
		m.cursor = m.sidebar.stepCursor(m.sidebarInput(), -1)
	case "u":
		m.cursor = nextNeedingAttention(m.sessions, m.cursor)
	case "b":
		m.hideSidebar = !m.hideSidebar
	case "z":
		return m, m.actions.toggleAFK()
	case "tab":
		m = m.toggleFocus()
		return m, nil
	case "q":
		return m, nav.Back()
	case "o":
		return m, nav.SwitchLast()
	case "ctrl+o":
		return m, nav.OpenSwitcher()
	case "?":
		return m.openHelp(), nil
	default:
		if len(k) == 1 && k[0] >= '1' && k[0] <= '8' {
			return m, nav.Switch(nav.ViewState{Tab: digitTabs[k[0]-'1']})
		}
		return m, nil
	}
	return m.refresh()
}

func (m Model) toggleFocus() Model {
	if m.terminalFocused {
		m.terminalFocused = false
	} else if m.attachTarget() != "" {
		m.terminalFocused = true
		m.attach.Resize(m.termView().ScreenSize())
	}
	return m
}

// nextNeedingAttention is the next session after the cursor needing
// attention; the cursor when there is none.
func nextNeedingAttention(sessions []wire.SessionInfo, cursor int) int {
	if i := attentionFrom(sessions, cursor+1); i >= 0 {
		return i
	}
	return cursor
}

// attentionFrom is the first session from start, wrapping, that is
// Actionable, or failing that Unread; -1 when there is none.
func attentionFrom(sessions []wire.SessionInfo, start int) int {
	for _, needs := range []func(wire.SessionInfo) bool{
		agentstyle.IsActionable,
		func(s wire.SessionInfo) bool { return s.Unread.Unread },
	} {
		for step := range len(sessions) {
			i := (start + step) % len(sessions)
			if needs(sessions[i]) {
				return i
			}
		}
	}
	return -1
}
