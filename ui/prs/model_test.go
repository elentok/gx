package prs

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func sendModel(m Model, msg tea.Msg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func TestModelRendersLoadingPlaceholder(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	content := m.View().Content
	if !strings.Contains(content, "loading") {
		t.Fatalf("expected loading placeholder content, got:\n%s", content)
	}
}

func TestModelRendersEmptyPlaceholder(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{})

	content := m.View().Content
	if !strings.Contains(content, "no PRs") {
		t.Fatalf("expected placeholder content, got:\n%s", content)
	}
}

func TestModelRendersPRRows(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{prs: []git.PR{
		{Number: 12, Title: "Add widget", UpdatedAt: time.Now()},
		{Number: 34, Title: "Draft feature", IsDraft: true, UpdatedAt: time.Now()},
	}})

	content := m.View().Content
	if !strings.Contains(content, "#12") || !strings.Contains(content, "Add widget") {
		t.Fatalf("expected PR #12 row, got:\n%s", content)
	}
	if !strings.Contains(content, "#34") || !strings.Contains(content, "DRAFT") {
		t.Fatalf("expected draft PR #34 with DRAFT badge, got:\n%s", content)
	}
}

func TestModelRendersLoadError(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{err: errBoom})

	content := m.View().Content
	if !strings.Contains(content, "error") {
		t.Fatalf("expected error content, got:\n%s", content)
	}
}

func TestOpenSelectedReturnsOpenURLCmd(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{prs: []git.PR{
		{Number: 12, Title: "Add widget", URL: "https://example.com/pull/12", UpdatedAt: time.Now()},
	}})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd from enter")
	}
	msg := cmd()
	if _, ok := msg.(gotoPRMsg); !ok {
		t.Fatalf("expected gotoPRMsg, got %T", msg)
	}
}

func TestQuestionMarkOpensHelpOverlay(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.help.IsOpen {
		t.Fatal("help should start closed")
	}

	m = sendModel(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !m.help.IsOpen {
		t.Fatal("expected help open after ?")
	}

	content := m.View().Content
	if !strings.Contains(content, "Keybindings") {
		t.Fatalf("expected help overlay with Keybindings title, got:\n%s", content)
	}
}

func TestQKeyReturnsBackCmd(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected a nav.Back() cmd from q")
	}
}

var errBoom = errTest("boom")

type errTest string

func (e errTest) Error() string { return string(e) }
