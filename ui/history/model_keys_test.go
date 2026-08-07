package history

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui/help"
)

func TestFooterHints_BuiltFromRealBindings(t *testing.T) {
	tests := []struct {
		name   string
		render func() string
		want   string
	}{
		{"projects", projectsFooterHint, "/ filter · enter open · q quit · ? help"},
		{"conversations", conversationsFooterHint, "/ filter · enter export+edit · ctrl+r resume · ctrl+y yank id · esc back · ? help"},
		{"grep", grepFooterHint, "enter export+edit · ctrl+r resume · ctrl+y yank id · ctrl+g toggle scope · esc back · ? help"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ansi.Strip(tt.render()); got != tt.want {
				t.Fatalf("%s footer hint = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestHelp_OpensAndListsCurrentPageBindings(t *testing.T) {
	m := newTestModel()
	m, _ = press(m, "?")
	if !m.help.IsOpen {
		t.Fatal("expected '?' to open the help overlay on the projects page")
	}
	if !sectionsHaveTitle(m.help.KeySections, "open project") {
		t.Fatalf("expected projects help sections to list 'open project', got %+v", m.help.KeySections)
	}
	if sectionsHaveTitle(m.help.KeySections, "resume") {
		t.Fatal("projects help sections should not list conversations-only bindings like 'resume'")
	}
}

func TestHelp_ReflectsConversationsPageBindings(t *testing.T) {
	m := enterConversationsFixture(t)
	m, _ = press(m, "?")
	if !m.help.IsOpen {
		t.Fatal("expected '?' to open the help overlay on the conversations page")
	}
	if !sectionsHaveTitle(m.help.KeySections, "resume") {
		t.Fatalf("expected conversations help sections to list 'resume', got %+v", m.help.KeySections)
	}
}

func TestHelp_ReflectsGrepPageBindings(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m, _ = press(m, "?")
	if !m.help.IsOpen {
		t.Fatal("expected '?' to open the help overlay on the grep page")
	}
	if !sectionsHaveTitle(m.help.KeySections, "toggle scope") {
		t.Fatalf("expected grep help sections to list 'toggle scope', got %+v", m.help.KeySections)
	}
}

func TestHelp_EscClosesOverlay(t *testing.T) {
	m := newTestModel()
	m, _ = press(m, "?")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.help.IsOpen {
		t.Fatal("expected esc to close the help overlay")
	}
	if m.page != pageProjects {
		t.Fatal("expected esc that closed help to not also navigate back")
	}
}

func sectionsHaveTitle(sections []help.KeySection, title string) bool {
	for _, s := range sections {
		for _, b := range s.Bindings {
			if strings.EqualFold(b.Title, title) {
				return true
			}
		}
	}
	return false
}
