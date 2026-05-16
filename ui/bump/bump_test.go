package bump

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/components"
)

func modelAtPick() Model {
	m := New()
	m.IsOpen = true
	m.phase = phasePick
	m.menu = components.MenuState{
		Items: []components.MenuItem{
			{Label: "patch", Detail: "v0.1.0 → v0.1.1"},
			{Label: "minor", Detail: "v0.1.0 → v0.2.0"},
			{Label: "major", Detail: "v0.1.0 → v1.0.0"},
		},
	}
	return m
}

func TestEscapeAtPickCancelsWithoutTag(t *testing.T) {
	m := modelAtPick()

	_, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if !result.Done {
		t.Fatal("expected Done=true on escape")
	}
	if result.NewTag != "" {
		t.Fatalf("expected NewTag to be empty on cancel, got %q", result.NewTag)
	}
	if result.Err != nil {
		t.Fatalf("expected no error on cancel, got %v", result.Err)
	}
}
