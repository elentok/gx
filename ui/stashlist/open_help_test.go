package stashlist

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/nav"
)

func TestOpenHelpOpensHelp(t *testing.T) {
	m := newReadyModel(t)

	if m = sendModel(m, nav.OpenHelp()()); !m.help.IsOpen {
		t.Fatal("expected help open")
	}
}

func TestOpenHelpFromDetailSearchEndsSearch(t *testing.T) {
	m := newReadyModel(t)
	m = sendModel(m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = sendModel(m, tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.commitDetail.InputFocused() {
		t.Fatal("expected detail search input focused after /")
	}

	m = sendModel(m, nav.OpenHelp()())
	if !m.help.IsOpen {
		t.Fatal("expected help open")
	}
	if m.commitDetail.InputFocused() {
		t.Fatal("expected detail search ended")
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("ending the search moved focus off the detail panel")
	}
}

func TestOpenHelpWithModalOpenDoesNothing(t *testing.T) {
	m := newReadyModel(t)
	m.stashCreate.IsOpen = true

	if m = sendModel(m, nav.OpenHelp()()); m.help.IsOpen {
		t.Fatal("expected help to stay closed while a modal is open")
	}
}
