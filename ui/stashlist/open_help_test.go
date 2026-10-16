package stashlist

import (
	"testing"

	"github.com/elentok/gx/ui/nav"
)

func TestOpenHelpOpensHelp(t *testing.T) {
	m := newReadyModel(t)

	if m = sendModel(m, nav.OpenHelp()()); !m.help.IsOpen {
		t.Fatal("expected help open")
	}
}

func TestOpenHelpWithModalOpenDoesNothing(t *testing.T) {
	m := newReadyModel(t)
	m.stashCreate.IsOpen = true

	if m = sendModel(m, nav.OpenHelp()()); m.help.IsOpen {
		t.Fatal("expected help to stay closed while a modal is open")
	}
}
