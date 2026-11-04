package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func TestModelUpdate_SearchLifecycle(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	// '/' activates search input mode
	next, _, result := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !result.Handled {
		t.Fatal("expected / to be handled by search")
	}
	if next.Search().Mode() != search.SearchModeInput {
		t.Fatalf("mode=%v want input", next.Search().Mode())
	}

	// Typing a query char: cmd must be nil (handled internally), matches computed
	next, cmd, result := next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if !result.Handled {
		t.Fatal("expected query key to be handled by search")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd (search handled internally), got %T", cmd)
	}
	// SearchMatchAt reports a match for rows containing "t"
	foundMatch := false
	for i := 0; i < len(next.data.ViewLines); i++ {
		if matched, _ := next.SearchMatchAt(i); matched {
			foundMatch = true
			break
		}
	}
	if !foundMatch {
		t.Fatal("expected SearchMatchAt to return matched=true for at least one row after typing query")
	}
}

func TestModelUpdate_SearchEnterSetsConfirmed(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	// Activate and type query
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})

	// Enter confirms search
	next, _, result := next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Handled {
		t.Fatal("expected enter to be handled")
	}
	if !result.SearchConfirmed {
		t.Fatal("expected SearchConfirmed=true after Enter")
	}
	if next.Search().Mode() != search.SearchModeResults {
		t.Fatalf("expected SearchModeResults after Enter, got %v", next.Search().Mode())
	}
}

func TestModelUpdate_SearchEscapeClearsMatches(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	// Activate and type query
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})

	// Verify we have matches before escaping
	foundBefore := false
	for i := 0; i < len(next.data.ViewLines); i++ {
		if matched, _ := next.SearchMatchAt(i); matched {
			foundBefore = true
			break
		}
	}
	if !foundBefore {
		t.Fatal("setup: expected matches before escape")
	}

	// Escape clears search
	next, _, result := next.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !result.Handled {
		t.Fatal("expected escape to be handled")
	}
	for i := 0; i < len(next.data.ViewLines); i++ {
		if matched, _ := next.SearchMatchAt(i); matched {
			t.Fatalf("expected SearchMatchAt(row=%d) to return false after Escape", i)
		}
	}
}

func TestModelUpdate_SearchNMovesCurrentMatch(t *testing.T) {
	m := NewModel(false)
	// Two hunks, each with a "match" line
	raw := "@@ -1,3 +1,3 @@\n-match one\n+match two\n context\n@@ -10,2 +10,2 @@\n-match three\n+context\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 20)

	// Activate search and type query that matches multiple rows
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})

	// Enter confirms so n/N navigation works
	next, _, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Find which row is currently "current"
	firstCurrent := -1
	for i := 0; i < len(next.data.ViewLines); i++ {
		if _, cur := next.SearchMatchAt(i); cur {
			firstCurrent = i
			break
		}
	}
	if firstCurrent < 0 {
		t.Fatal("expected a current match after Enter")
	}

	// 'n' moves to next match — current row should differ
	after, _, result := next.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !result.Handled {
		t.Fatal("expected n to be handled")
	}
	newCurrent := -1
	for i := 0; i < len(after.data.ViewLines); i++ {
		if _, cur := after.SearchMatchAt(i); cur {
			newCurrent = i
			break
		}
	}
	if newCurrent == firstCurrent {
		t.Fatalf("expected current match to move after n, still at row %d", firstCurrent)
	}
}

func TestModelBuildFromRaw_RecomputesMatchesOnReload(t *testing.T) {
	m := NewModel(false)
	raw1 := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw1, raw1)
	m.SyncViewport(80, 10)

	// Activate search and type query
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	countBefore := next.search.MatchesCount()
	if countBefore == 0 {
		t.Fatal("setup: expected matches after typing query")
	}

	// Load new diff content — matches should be recomputed automatically
	raw2 := "@@ -1,2 +1,2 @@\n-text alpha\n+text beta\n-text gamma\n+text delta\n"
	next.BuildFromRaw(raw2, raw2)
	countAfter := next.search.MatchesCount()
	if countAfter == 0 {
		t.Fatal("expected matches to be recomputed after BuildFromRaw with active query")
	}
	// New diff has more "t" matches than the old one
	if countAfter <= countBefore {
		t.Fatalf("expected more matches after reload (got %d <= %d)", countAfter, countBefore)
	}
}

func TestModelRenderRows_SearchBoxAppearsWhenInputActive(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	bodyH := 10
	// Activate search input
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !next.search.InputFocused() {
		t.Fatal("setup: expected InputFocused after /")
	}

	lines := next.RenderRows(bodyH, true, RenderOpts{InnerWidth: 80})
	if len(lines) != bodyH {
		t.Fatalf("RenderRows returned %d lines, want %d", len(lines), bodyH)
	}
	// The last non-empty line should contain search box content (border char)
	lastContent := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(ansi.Strip(lines[i])) != "" {
			lastContent = lines[i]
			break
		}
	}
	if !strings.Contains(ansi.Strip(lastContent), "Search") && !strings.Contains(lastContent, "╯") && !strings.Contains(lastContent, "─") {
		t.Errorf("expected bottom lines to contain search box, last non-empty line: %q", ansi.Strip(lastContent))
	}
}

func TestModelRenderRows_SearchBoxGoneAfterEnter(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	bodyH := 10

	// Activate, type a query, then confirm with Enter
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	next, _, result := next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.SearchConfirmed {
		t.Fatal("setup: expected SearchConfirmed after Enter")
	}
	if next.search.InputFocused() {
		t.Fatal("setup: expected InputFocused=false after Enter")
	}

	lines := next.RenderRows(bodyH, true, RenderOpts{InnerWidth: 80})
	if len(lines) != bodyH {
		t.Fatalf("RenderRows returned %d lines after Enter, want %d", len(lines), bodyH)
	}
	// None of the lines should contain the search input box title "Search"
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), "Search") {
			t.Errorf("line %d still contains search box after Enter: %q", i, ansi.Strip(line))
		}
	}
}
