package log

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
)

func sizedLP(w, h int) listPanel {
	m := newListPanel()
	m.width = w
	m.height = h
	return m
}

func commitRows(n int) []row {
	rows := make([]row, n)
	for i := range rows {
		rows[i] = row{kind: rowCommit, commit: git.LogEntry{
			FullHash:    fmt.Sprintf("hash%04d", i),
			Hash:        fmt.Sprintf("hash%04d", i)[:8],
			Subject:     fmt.Sprintf("commit %d", i),
			AuthorShort: "AB",
		}}
	}
	return rows
}

// --- SelectedRef ---

func TestListPanelEmptyRowsReturnsEmptyRef(t *testing.T) {
	m := newListPanel()
	if got := m.SelectedRef(); got != "" {
		t.Fatalf("SelectedRef on empty = %q, want ''", got)
	}
}

func TestListPanelPseudoStatusRowReturnsEmptyRef(t *testing.T) {
	m := newListPanel().WithRows([]row{{kind: rowPseudoStatus, detail: "clean"}})
	if got := m.SelectedRef(); got != "" {
		t.Fatalf("SelectedRef on pseudo-status = %q, want ''", got)
	}
}

func TestListPanelSelectedRefReturnsFullHash(t *testing.T) {
	rows := []row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa111bbb222", Subject: "first"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "ccc333ddd444", Subject: "second"}},
	}
	m := sizedLP(80, 20).WithRows(rows)
	if got := m.SelectedRef(); got != "aaa111bbb222" {
		t.Fatalf("initial SelectedRef = %q, want 'aaa111bbb222'", got)
	}
	m = m.SetSelected(1)
	if got := m.SelectedRef(); got != "ccc333ddd444" {
		t.Fatalf("after SetSelected(1) SelectedRef = %q, want 'ccc333ddd444'", got)
	}
}

// --- Navigation ---

func TestListPanelNavigateChangesSelectionAndRef(t *testing.T) {
	rows := []row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa", Subject: "a"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "bbb", Subject: "b"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "ccc", Subject: "c"}},
	}
	m := sizedLP(80, 20).WithRows(rows)

	if m.Selected() != 0 {
		t.Fatalf("initial Selected = %d, want 0", m.Selected())
	}

	m = m.Navigate(1)
	if m.Selected() != 1 {
		t.Fatalf("Navigate(1): Selected = %d, want 1", m.Selected())
	}
	if m.SelectedRef() != "bbb" {
		t.Fatalf("Navigate(1): SelectedRef = %q, want 'bbb'", m.SelectedRef())
	}

	m = m.Navigate(1)
	m = m.Navigate(-1)
	if m.Selected() != 1 {
		t.Fatalf("Navigate(+1,-1) from 1: Selected = %d, want 1", m.Selected())
	}
}

func TestListPanelNavigateClampedAtEdges(t *testing.T) {
	rows := []row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa", Subject: "a"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "bbb", Subject: "b"}},
	}
	m := sizedLP(80, 20).WithRows(rows)

	m = m.Navigate(-5)
	if m.Selected() != 0 {
		t.Fatalf("Navigate(-5) from 0: Selected = %d, want 0", m.Selected())
	}

	m = m.Navigate(100)
	if m.Selected() != 1 {
		t.Fatalf("Navigate(100) past end: Selected = %d, want 1", m.Selected())
	}
}

func TestListPanelScrollPageAdvancesSelection(t *testing.T) {
	m := sizedLP(80, 20).WithRows(commitRows(30))
	before := m.Selected()
	m = m.ScrollPage(1)
	if m.Selected() <= before {
		t.Fatalf("ScrollPage(1): expected selection > %d, got %d", before, m.Selected())
	}
}

// --- Container focus colors ---

func TestListPanelContainerFocusControlsColors(t *testing.T) {
	active := newListPanel().WithContainerFocus(true)
	if active.frameBorderColor() != ui.ColorOrange {
		t.Fatalf("active border = %v, want ColorOrange", active.frameBorderColor())
	}
	if active.frameTitleColor() != ui.ColorOrange {
		t.Fatalf("active title = %v, want ColorOrange", active.frameTitleColor())
	}

	inactive := newListPanel().WithContainerFocus(false)
	if inactive.frameBorderColor() != ui.ColorBorder {
		t.Fatalf("inactive border = %v, want ColorBorder", inactive.frameBorderColor())
	}
	if inactive.frameTitleColor() != ui.ColorBlue {
		t.Fatalf("inactive title = %v, want ColorBlue", inactive.frameTitleColor())
	}
}

// --- Render hints: search highlight ---

func TestListPanelSearchHighlightPresent(t *testing.T) {
	r := row{
		kind:   rowCommit,
		commit: git.LogEntry{Hash: "abcdef1", Subject: "fix the bug", AuthorShort: "AB"},
	}
	m := sizedLP(80, 20).WithRows([]row{r})
	hints := listPanelHints{
		highlight: func(text string) string {
			const q = "fix"
			if idx := strings.Index(strings.ToLower(text), q); idx >= 0 {
				return text[:idx] + logSearchStyle.Render(text[idx:idx+len(q)]) + text[idx+len(q):]
			}
			return text
		},
	}
	line := m.WithHints(hints).renderCommitRow(r, false)
	stripped := ansi.Strip(line)
	if !strings.Contains(stripped, "fix the bug") {
		t.Fatalf("stripped row missing subject: %q", stripped)
	}
	if line == stripped {
		t.Fatal("expected ANSI highlight when search matches — row rendered as plain text")
	}
}

// --- Render hints: flash ---

func TestListPanelFlashedRowHasAnsiDecoration(t *testing.T) {
	r := row{
		kind:   rowCommit,
		commit: git.LogEntry{Hash: "abcdef1", FullHash: "abcdef111", Subject: "my subject", AuthorShort: "AB"},
	}
	m := sizedLP(80, 20).WithRows([]row{r})
	hints := listPanelHints{
		flashSubject: "my subject",
		flashUntil:   time.Now().Add(5 * time.Second),
	}
	line := m.WithHints(hints).renderRow(r, false, 40)
	stripped := ansi.Strip(line)
	if !strings.Contains(stripped, "my subject") {
		t.Fatalf("flashed row stripped %q missing 'my subject'", stripped)
	}
	if line == stripped {
		t.Fatal("expected ANSI background on flashed row — rendered as plain text")
	}
}

func TestListPanelExpiredFlashIsNotApplied(t *testing.T) {
	r := row{
		kind:   rowCommit,
		commit: git.LogEntry{Hash: "abcdef1", FullHash: "abcdef111", Subject: "my subject", AuthorShort: "AB"},
	}
	m := sizedLP(80, 20).WithRows([]row{r})
	hints := listPanelHints{
		flashSubject: "my subject",
		flashUntil:   time.Now().Add(-1 * time.Second), // already expired
	}
	line := m.WithHints(hints).renderRow(r, false, 40)
	// Non-selected + expired flash should not have the flash background.
	// We can't guarantee no ANSI (the row styles may still apply), but it
	// must at least not match a flashed selected row.
	flashedLine := m.WithHints(listPanelHints{
		flashSubject: "my subject",
		flashUntil:   time.Now().Add(5 * time.Second),
	}).renderRow(r, false, 40)
	if line == flashedLine {
		t.Fatal("expired flash should render differently from active flash")
	}
}

// --- Render hints: decorations ---

func TestListPanelDecorationsRenderedAsBadges(t *testing.T) {
	r := row{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "abcdef1",
			Subject:     "subject",
			AuthorShort: "AB",
			Decorations: []git.RefDecoration{{Name: "origin/main", Kind: git.RefDecorationRemoteBranch}},
		},
	}
	m := sizedLP(80, 20).WithRows([]row{r})
	line := m.renderBadges(r.commit.Decorations, false)
	if !strings.Contains(ansi.Strip(line), "origin/main") {
		t.Fatalf("badges %q missing 'origin/main'", ansi.Strip(line))
	}
}

func TestListPanelHiddenRefOmittedFromBadges(t *testing.T) {
	import_re := compileHideRefs([]string{"refs/heads/.*"})
	r := row{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "abcdef1",
			Subject:     "s",
			AuthorShort: "AB",
			Decorations: []git.RefDecoration{
				{Name: "refs/heads/main", Kind: git.RefDecorationLocalBranch},
				{Name: "origin/main", Kind: git.RefDecorationRemoteBranch},
			},
		},
	}
	m := sizedLP(80, 20).WithRows([]row{r})
	hints := listPanelHints{compiledHideRefs: import_re}
	line := m.WithHints(hints).renderBadges(r.commit.Decorations, false)
	stripped := ansi.Strip(line)
	if strings.Contains(stripped, "refs/heads/main") {
		t.Fatalf("hidden ref should be omitted, got %q", stripped)
	}
	if !strings.Contains(stripped, "origin/main") {
		t.Fatalf("non-hidden ref missing from %q", stripped)
	}
}

// --- Condensed rows ---

func TestRenderRowUsesShortDateAndSingleSpaceGapForBadges(t *testing.T) {
	r := row{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "abcdef1",
			Subject:     "subject",
			AuthorShort: "AB",
			Date:        time.Now().Add(-2 * time.Hour),
			Decorations: []git.RefDecoration{{Name: "origin/main", Kind: git.RefDecorationRemoteBranch}},
		},
	}
	m := sizedLP(80, 20).WithRows([]row{r})

	condensed := ansi.Strip(m.renderRow(r, false, ui.NarrowWidthThreshold-1))
	wide := ansi.Strip(m.renderRow(r, false, ui.NarrowWidthThreshold+50))

	if !strings.Contains(condensed, "2h ") || strings.Contains(condensed, "2h ago") {
		t.Fatalf("condensed row should show short date without 'ago', got %q", condensed)
	}
	if !strings.Contains(wide, "2h ago") {
		t.Fatalf("wide row should keep full 'ago' date, got %q", wide)
	}
	if !strings.Contains(condensed, "origin/main subject") {
		t.Fatalf("condensed row should have single-space gap after badges, got %q", condensed)
	}
	if !strings.Contains(wide, "origin/main subject") {
		t.Fatalf("wide row should have single-space gap after badges, got %q", wide)
	}
}

func TestRenderBadgesMergesIntoGroupWhenCondensed(t *testing.T) {
	decorations := []git.RefDecoration{
		{Name: "main", Kind: git.RefDecorationLocalBranch},
		{Name: "origin/main", Kind: git.RefDecorationRemoteBranch},
	}
	m := sizedLP(80, 20)

	condensed := m.renderBadges(decorations, true)
	separate := m.renderBadges(decorations, false)

	if ansi.Strip(condensed) != ansi.Strip(separate) {
		t.Fatalf("condensed and separate badges should show the same names, got %q vs %q", ansi.Strip(condensed), ansi.Strip(separate))
	}
	if condensed == separate {
		t.Fatalf("expected condensed (merged) and separate badge rendering to differ in styling")
	}
}

func TestRenderBadgesSingleDecorationUnchangedWhenCondensed(t *testing.T) {
	decorations := []git.RefDecoration{{Name: "main", Kind: git.RefDecorationLocalBranch}}
	m := sizedLP(80, 20)

	condensed := m.renderBadges(decorations, true)
	separate := m.renderBadges(decorations, false)

	if condensed != separate {
		t.Fatalf("single-decoration badge should render identically condensed or not: %q vs %q", condensed, separate)
	}
}

// --- Pseudo-status row ---

func TestListPanelPseudoStatusRowShowsWorkingTree(t *testing.T) {
	r := row{kind: rowPseudoStatus, detail: ""}
	m := sizedLP(80, 20).WithRows([]row{r})
	line := m.renderRow(r, false, 76)
	if !strings.Contains(ansi.Strip(line), "working tree") {
		t.Fatalf("pseudo-status line %q missing 'working tree'", ansi.Strip(line))
	}
}
