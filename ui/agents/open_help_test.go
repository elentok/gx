package agents

import (
	"testing"

	"github.com/elentok/gx/ui/nav"
)

func TestOpenHelp_TerminalFocusedOpensHelpAndSendsNothing(t *testing.T) {
	m, a := focusedTerminal(t)
	m = update1(t, m, ctrlA)
	m = update1(t, m, nav.OpenHelp()())
	if !m.help.IsOpen {
		t.Fatal("OpenHelp did not open help")
	}
	if m.leaderPending {
		t.Fatal("OpenHelp left leader mode pending")
	}
	if len(a.keys) != 0 || len(a.pastes) != 0 {
		t.Fatalf("OpenHelp sent keys %v and pastes %v to the session", a.keys, a.pastes)
	}
}

func TestOpenHelp_IgnoredWhileModalOrCopyModeOpen(t *testing.T) {
	m, _ := focusedTerminal(t)
	m.menu.IsOpen = true
	if update1(t, m, nav.OpenHelp()()).help.IsOpen {
		t.Fatal("OpenHelp opened help over the menu")
	}

	m, _ = focusedTerminal(t)
	m, _ = leaderThen(t, m, letter("["))
	if m.copy == nil {
		t.Fatal("leader [ did not enter copy mode")
	}
	if update1(t, m, nav.OpenHelp()()).help.IsOpen {
		t.Fatal("OpenHelp opened help in copy mode")
	}
}
