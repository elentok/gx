package prs

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
)

func TestOpenHelp_OpensHelpOverlay(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, nav.OpenHelp()())
	if !m.help.IsOpen {
		t.Fatal("OpenHelp did not open help")
	}
}

func TestOpenHelp_IgnoredWhileCommentsOpen(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m.comments.isOpen = true
	m = sendModel(m, nav.OpenHelp()())
	if m.help.IsOpen {
		t.Fatal("OpenHelp opened help over the comments modal")
	}
}
