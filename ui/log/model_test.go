package log

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestEnterOnPseudoRowOpensStatus(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "dirty.txt", "dirty\n")

	m := New(repo, "")
	if len(m.rows) == 0 || m.rows[0].kind != rowPseudoStatus {
		t.Fatalf("expected pseudo status row first")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected nav command on enter")
	}
	route, ok := nav.IsPush(cmd())
	if !ok {
		t.Fatalf("expected nav push")
	}
	if route.Kind != nav.RouteStatus {
		t.Fatalf("expected status route, got %q", route.Kind)
	}
	_ = updated
}

func TestGHResetsCustomRefToHead(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "one")

	m := NewWithSettings(repo, "HEAD~1", Settings{EnableNavigation: true})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if cmd == nil {
		t.Fatalf("expected nav command for gh")
	}
	route, ok := nav.IsReplace(cmd())
	if !ok {
		t.Fatalf("expected nav replace")
	}
	if route.Kind != nav.RouteLog || route.Ref != "HEAD" {
		t.Fatalf("expected log HEAD route, got kind=%q ref=%q", route.Kind, route.Ref)
	}
}

func TestEnterOnCommitRowOpensCommitRoute(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := New(repo, "")
	for i := range m.rows {
		if m.rows[i].kind == rowCommit {
			m.cursor = i
			break
		}
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected nav command on enter")
	}
	route, ok := nav.IsPush(cmd())
	if !ok {
		t.Fatalf("expected nav push")
	}
	if route.Kind != nav.RouteCommit {
		t.Fatalf("expected commit route, got %q", route.Kind)
	}
	_ = updated
}

func TestSelectedCommitRowFillsFullWidth(t *testing.T) {
	m := Model{
		width: 80,
		rows: []row{{
			kind: rowCommit,
			commit: git.LogEntry{
				Hash:        "12345678",
				AuthorShort: "AB",
				Subject:     "subject",
				Date:        time.Now().Add(-2 * time.Hour),
				Graph:       "*",
				Decorations: []git.RefDecoration{{Name: "main", Kind: git.RefDecorationLocalBranch}},
			},
		}},
	}
	line := m.renderRow(m.rows[0], true, 40)
	if got := ansi.StringWidth(ansi.Strip(line)); got != 40 {
		t.Fatalf("selected row width = %d, want 40", got)
	}
}

func TestBadgeVariantForDecoration(t *testing.T) {
	tests := []struct {
		name string
		dec  git.RefDecoration
		want ui.BadgeVariant
	}{
		{name: "main local branch", dec: git.RefDecoration{Name: "main", Kind: git.RefDecorationLocalBranch}, want: ui.BadgeVariantYellow},
		{name: "main remote branch", dec: git.RefDecoration{Name: "origin/main", Kind: git.RefDecorationRemoteBranch}, want: ui.BadgeVariantYellow},
		{name: "feature branch", dec: git.RefDecoration{Name: "feature/x", Kind: git.RefDecorationLocalBranch}, want: ui.BadgeVariantMauve},
		{name: "tag", dec: git.RefDecoration{Name: "v1.0.0", Kind: git.RefDecorationTag}, want: ui.BadgeVariantBlue},
	}

	for _, tt := range tests {
		if got := badgeVariantForDecoration(tt.dec); got != tt.want {
			t.Fatalf("%s: variant = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestRenderCommitRowHighlightsSearchMatches(t *testing.T) {
	m := Model{
		searchQuery: "fix",
		rows: []row{{
			kind: rowCommit,
			commit: git.LogEntry{
				Hash:        "12345678",
				AuthorShort: "AB",
				Subject:     "fix search highlighting",
			},
		}},
	}

	line := m.renderCommitRow(m.rows[0])
	if stripped := ansi.Strip(line); !strings.Contains(stripped, "fix search highlighting") {
		t.Fatalf("stripped line = %q", stripped)
	}
	if line == ansi.Strip(line) {
		t.Fatalf("expected ansi highlight in rendered commit row")
	}
}

func TestRenderBadgesHighlightsSearchMatches(t *testing.T) {
	m := Model{searchQuery: "main"}
	line := m.renderBadges([]git.RefDecoration{{Name: "origin/main", Kind: git.RefDecorationRemoteBranch}})
	if stripped := ansi.Strip(line); !strings.Contains(stripped, "origin/main") {
		t.Fatalf("stripped badges = %q", stripped)
	}
	if line == ansi.Strip(line) {
		t.Fatalf("expected ansi highlight in rendered badges")
	}
}

func TestCloseSearchKeepsMatchesVisible(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "fix one")
	testutil.WriteFile(t, repo, "two.txt", "two\n")
	testutil.CommitAll(t, repo, "fix two")

	m := New(repo, "")
	m.enterSearchMode()
	m.searchQuery = "fix"
	m.recomputeSearchMatches()
	if len(m.searchMatch) == 0 {
		t.Fatalf("expected search matches")
	}

	m.closeSearch()
	if m.searchQuery != "fix" {
		t.Fatalf("expected query to persist after close, got %q", m.searchQuery)
	}
	if len(m.searchMatch) == 0 {
		t.Fatalf("expected matches to persist after close")
	}
}

func TestNAndNShiftMoveBetweenSearchResults(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "fix one")
	testutil.WriteFile(t, repo, "two.txt", "two\n")
	testutil.CommitAll(t, repo, "fix two")

	m := New(repo, "")
	m.searchQuery = "fix"
	m.recomputeSearchMatches()
	if len(m.searchMatch) < 2 {
		t.Fatalf("expected at least two search matches, got %d", len(m.searchMatch))
	}
	m.cursor = m.searchMatch[0]
	m.searchCursor = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.cursor != m.searchMatch[1] {
		t.Fatalf("expected n to move to next result")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N', Mod: tea.ModShift})
	m = updated.(Model)
	if m.cursor != m.searchMatch[0] {
		t.Fatalf("expected N to move to previous result")
	}
}
