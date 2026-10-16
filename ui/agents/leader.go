package agents

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
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
	{Seq: []string{"n"}, Title: "next session"},
	{Seq: []string{"p"}, Title: "previous session"},
	{Seq: []string{"u"}, Title: "next needing attention"},
	{Seq: []string{"b"}, Title: "toggle sidebar"},
	{Seq: []string{"tab"}, Title: "switch focus"},
	{Seq: []string{"1-8"}, Title: "switch gx tab"},
	{Seq: []string{"q"}, Title: "leave tab"},
	{Seq: []string{"?"}, Title: "help"},
}

var sidebarBindings = []keys.Binding{
	{Seq: []string{"j"}, Display: "j/↓", Title: "next session"},
	{Seq: []string{"k"}, Display: "k/↑", Title: "previous session"},
	{Seq: []string{"enter"}, Title: "focus terminal"},
	{Seq: []string{"u"}, Title: "next needing attention"},
	{Seq: []string{"/"}, Title: "jump to session"},
	{Seq: []string{"c"}, Title: "new session"},
	{Seq: []string{"b"}, Title: "toggle sidebar"},
}

func (m Model) leaderKeys() []string {
	if len(m.settings.Agents.LeaderKey) == 0 {
		return []string{config.DefaultLeaderKey}
	}
	return m.settings.Agents.LeaderKey
}

// updateKey matches the leader before anything else, in either focus, so the
// leader never reaches the session on its own.
func (m Model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	isLeader := slices.Contains(m.leaderKeys(), msg.String())
	switch {
	case m.help.IsOpen:
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	case m.jump != nil:
		return m.updateJumpKey(msg)
	case m.create != nil:
		return m.updateCreateKey(msg)
	case m.leaderPending:
		m.leaderPending = false
		if isLeader {
			m.sendKey(msg)
			return m, nil
		}
		return m.runLeaderAction(msg.String())
	case isLeader:
		m.leaderPending = true
		return m, nil
	case m.terminalFocused:
		m.sendKey(msg)
		return m, nil
	}
	return m.updateSidebarKey(msg)
}

// runLeaderAction runs the key pressed after the leader; esc and unbound keys
// are dropped.
func (m Model) runLeaderAction(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "j":
		m.jump = &jumpPopup{}
		return m, nil
	case "c":
		return m.openCreate()
	case "n":
		m.cursor = min(m.cursor+1, max(0, len(m.sessions)-1))
	case "p":
		m.cursor = max(m.cursor-1, 0)
	case "u":
		m.cursor = m.nextNeedingAttention()
	case "b":
		m.hideSidebar = !m.hideSidebar
	case "tab":
		m = m.toggleFocus()
		return m, nil
	case "q":
		return m, nav.Back()
	case "?":
		m.help = help.NewModel([]help.KeySection{
			help.NewKeySection("Leader ("+m.leaderKeys()[0]+" then)", leaderBindings...),
			help.NewKeySection("Sidebar", sidebarBindings...),
		})
		m.help.Open(m.width, m.height)
		return m, nil
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
		m.resizeAttach()
	}
	return m
}

// nextNeedingAttention is the next session after the cursor needing
// attention; the cursor when there is none.
func (m Model) nextNeedingAttention() int {
	if i := m.attentionFrom(m.cursor + 1); i >= 0 {
		return i
	}
	return m.cursor
}

// attentionFrom is the first session from start, wrapping, that is
// Actionable, or failing that Unread; -1 when there is none.
func (m Model) attentionFrom(start int) int {
	for _, needs := range []func(server.SessionInfo) bool{
		isActionable,
		func(s server.SessionInfo) bool { return s.Unread.Unread },
	} {
		for step := range len(m.sessions) {
			i := (start + step) % len(m.sessions)
			if needs(m.sessions[i]) {
				return i
			}
		}
	}
	return -1
}
