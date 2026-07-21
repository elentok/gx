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

func TestModelRendersNoPRsFoundPlaceholder(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{})

	content := m.View().Content
	if !strings.Contains(content, "no PRs found") {
		t.Fatalf("expected 'no PRs found' placeholder, got:\n%s", content)
	}
}

func TestModelRendersNoOpenPRsPlaceholder(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{anyPRs: true})

	content := m.View().Content
	if !strings.Contains(content, "no open PRs") {
		t.Fatalf("expected 'no open PRs' placeholder, got:\n%s", content)
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

func TestModelRendersFacetsAndMarker(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{prs: []git.PR{
		{
			Number:            12,
			Title:             "Add widget",
			UpdatedAt:         time.Now(),
			StatusCheckRollup: []git.PRStatusCheck{{Status: "COMPLETED", Conclusion: "SUCCESS"}},
			ReviewDecision:    "APPROVED",
			Mergeable:         "MERGEABLE",
			Reviews:           []git.PRReview{{Body: "lgtm"}},
		},
	}})

	content := m.View().Content
	if !strings.Contains(content, "1c") {
		t.Fatalf("expected comment count facet, got:\n%s", content)
	}
}

func TestModelRendersMergeConflictFacet(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{prs: []git.PR{
		{Number: 12, Title: "Add widget", UpdatedAt: time.Now(), Mergeable: "CONFLICTING"},
	}})

	content := m.View().Content
	if !strings.Contains(content, "⚠") {
		t.Fatalf("expected conflict marker facet, got:\n%s", content)
	}
}

func TestModelRendersLoadError(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{err: errBoom})

	content := m.View().Content
	if !strings.Contains(content, "error") || !strings.Contains(content, "boom") {
		t.Fatalf("expected raw wrapped error content, got:\n%s", content)
	}
}

func TestModelRendersGHNotInstalledError(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{err: &git.PRListError{Kind: git.PRListErrorGHNotInstalled, Err: errBoom}})

	content := m.View().Content
	if !strings.Contains(content, "gh not found") || !strings.Contains(content, "install") {
		t.Fatalf("expected gh-not-installed hint, got:\n%s", content)
	}
}

func TestModelRendersGHUnauthenticatedError(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{err: &git.PRListError{Kind: git.PRListErrorUnauthenticated, Err: errBoom}})

	content := m.View().Content
	if !strings.Contains(content, "not authenticated") || !strings.Contains(content, "gh auth login") {
		t.Fatalf("expected gh-unauthenticated hint, got:\n%s", content)
	}
}

func TestModelRendersClosedPRSection(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{closedPRs: []git.ClosedPR{
		{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now()},
		{Number: 6, Title: "Abandoned idea", State: "CLOSED", ClosedAt: time.Now()},
	}})

	content := m.View().Content
	if !strings.Contains(content, "Closed (last 2 weeks)") {
		t.Fatalf("expected closed-PR section header, got:\n%s", content)
	}
	if !strings.Contains(content, "Merged fix") || !strings.Contains(content, "Abandoned idea") {
		t.Fatalf("expected closed PR titles, got:\n%s", content)
	}
}

func TestModelRendersClosedSectionEmptyState(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{anyPRs: true})

	content := m.View().Content
	if !strings.Contains(content, "no recently closed PRs") {
		t.Fatalf("expected closed-PR empty state, got:\n%s", content)
	}
}

func TestModelRendersClosedSectionWhenOpenListEmpty(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{closedPRs: []git.ClosedPR{
		{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now()},
	}})

	content := m.View().Content
	if !strings.Contains(content, "no PRs found") {
		t.Fatalf("expected open-list empty state, got:\n%s", content)
	}
	if !strings.Contains(content, "Merged fix") {
		t.Fatalf("expected closed section to render alongside empty open list, got:\n%s", content)
	}
}

func TestModelRendersClosedSectionWhenOpenListErrors(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, prsLoadedMsg{
		err: errBoom,
		closedPRs: []git.ClosedPR{
			{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now()},
		},
	})

	content := m.View().Content
	if !strings.Contains(content, "boom") {
		t.Fatalf("expected open-list error, got:\n%s", content)
	}
	if !strings.Contains(content, "Merged fix") {
		t.Fatalf("expected closed section to render alongside erroring open list, got:\n%s", content)
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

func TestRKeyTriggersRefresh(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd == nil {
		t.Fatal("expected a refresh cmd from R")
	}
}

func TestRefreshMenuChordTriggersRefresh(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m = sendModel(m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("expected a refresh cmd from m r")
	}
}

func TestOnPageActivatedTriggersRefetch(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	cmd := m.OnPageActivated()
	if cmd == nil {
		t.Fatal("expected OnPageActivated to return a refetch cmd")
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
