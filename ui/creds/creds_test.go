package creds

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/components"
)

func TestCredsNew(t *testing.T) {
	m := New()
	if m.IsOpen {
		t.Error("expected IsOpen=false initially")
	}
}

func TestCredsOpen(t *testing.T) {
	m := New()
	m.Open(components.CredentialPrompt{Text: "Password:", Kind: components.PromptKindSecret})
	if !m.IsOpen {
		t.Error("expected IsOpen=true after Open")
	}
}

func TestCredsEsc(t *testing.T) {
	m := New()
	m.Open(components.CredentialPrompt{Text: "Token:"})
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !result.Decided || !result.Cancelled {
		t.Error("expected Decided=true, Cancelled=true on esc")
	}
	if next.IsOpen {
		t.Error("expected IsOpen=false after esc")
	}
}

func TestCredsEnter(t *testing.T) {
	m := New()
	m.Open(components.CredentialPrompt{Text: "Token:"})
	_, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Decided || result.Cancelled {
		t.Error("expected Decided=true, Cancelled=false on enter")
	}
}

func TestCredsView(t *testing.T) {
	m := New()
	m.Open(components.CredentialPrompt{Text: "Enter password:"})
	view := m.View(60)
	if view == "" {
		t.Error("expected non-empty view")
	}
}
