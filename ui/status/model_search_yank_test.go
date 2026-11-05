package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/status/diffarea"
	"strings"
	"testing"
)

func TestYankFilenameWithYFInFiletreeView(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)

	if got != "a.txt" {
		t.Fatalf("expected yanked filename, got %q", got)
	}
}

func TestYankLocationWithYLInFiletreeViewYanksFilenameOnly(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "b.txt", "one\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)

	if got != "@b.txt" {
		t.Fatalf("expected yl in filetree to yank filename only, got %q", got)
	}
}

func TestYankAllContextWithYAInDiffLineModeIncludesLocationAndSelectedLine(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "c.txt", "old-1\nold-2\n")
	testutil.MustGitExported(t, repo, "add", "c.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "c.txt", "new-1\nnew-2\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	if !strings.Contains(got, "@c.txt L") {
		t.Fatalf("expected ya output to include file and line, got %q", got)
	}
	if !strings.Contains(got, "-old-2") {
		t.Fatalf("expected ya output to include selected line content, got %q", got)
	}
	if strings.Contains(got, "-old-1") || strings.Contains(got, "+new") {
		t.Fatalf("expected ya output to exclude non-selected line content, got %q", got)
	}
}

func TestYankFilenameWithYFInDiffLineModeYanksFilenameOnly(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "d.txt", "old-1\nold-2\n")
	testutil.MustGitExported(t, repo, "add", "d.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "d.txt", "new-1\nnew-2\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)

	if got != "d.txt" {
		t.Fatalf("expected yf output to be filename only, got %q", got)
	}
}

func TestYankLocationOnlyWithYLInDiffVisualModeYanksOnlyLocation(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "e.txt", "old-1\nold-2\n")
	testutil.MustGitExported(t, repo, "add", "e.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "e.txt", "new-1\nnew-2\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)

	if !strings.HasPrefix(got, "@e.txt L") {
		t.Fatalf("expected yl output to include only file and line range, got %q", got)
	}
	if strings.Contains(got, "\n") || strings.Contains(got, "-old") || strings.Contains(got, "+new") {
		t.Fatalf("expected yl output to exclude content lines, got %q", got)
	}
}

func TestYankSelectionOnlyWithYYInDiffLineMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "f.txt", "old-1\nold-2\n")
	testutil.MustGitExported(t, repo, "add", "f.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "f.txt", "new-1\nnew-2\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	if strings.Contains(got, "@f.txt") {
		t.Fatalf("expected yy output to be raw selection only, got %q", got)
	}
	if got != "-old-1" {
		t.Fatalf("expected yy output to yank only active line, got %q", got)
	}
}

func TestStageSearchFiletreeModeAndNavigation(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "apple.txt", "one\n")
	testutil.WriteFile(t, repo, "apricot.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.filetree.Model.Search().Mode() != search.SearchModeInput {
		t.Fatalf("expected search input mode after /")
	}

	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"})
	if m.filetree.Model.Search().MatchesCount() < 2 {
		t.Fatalf("expected multiple filetree search matches, got %d", m.filetree.Model.Search().MatchesCount())
	}

	first := m.filetree.Data.ListState.Selected()
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.filetree.Model.Search().Mode() != search.SearchModeResults || !m.filetree.Model.Search().HasQuery() || m.filetree.Model.Search().MatchesCount() == 0 {
		t.Fatalf("expected enter to show search results mode while keeping highlights")
	}
	if pane := ansi.Strip(m.renderFiletreePane(40, 10)); !strings.Contains(pane, "1/2") {
		t.Fatalf("expected persistent search counter in filetree frame, got %q", pane)
	}

	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.filetree.Data.ListState.Selected() == first {
		t.Fatalf("expected n to move to next search result")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.filetree.Model.Search().Mode() != search.SearchModeNone || m.filetree.Model.Search().HasQuery() || m.filetree.Model.Search().MatchesCount() != 0 {
		t.Fatalf("expected esc to clear active search state")
	}
}

func TestFiletreeSearchModeShowsSearchBox(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "apple.txt", "one\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 60
	m.height = 20
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	lines := m.visibleStatusLines(30, 20)
	combined := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(combined, "Search") {
		t.Fatalf("expected rendered lines to contain 'Search', got %q", combined)
	}
}

func TestStageSearchDiffModeAndPrevNextKeys(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "needle-one\nline\nneedle-two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.diffarea.ActiveSectionModel().Search().Mode() != search.SearchModeInput {
		t.Fatalf("expected diff search input mode after /")
	}

	for _, r := range []rune{'n', 'e', 'e', 'd', 'l', 'e'} {
		m = runStatusCmds(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if m.diffarea.ActiveSectionModel().Search().MatchesCount() < 2 {
		t.Fatalf("expected multiple diff search matches, got %d", m.diffarea.ActiveSectionModel().Search().MatchesCount())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.diffarea.ActiveSectionModel().Search().Mode() != search.SearchModeResults || !m.diffarea.ActiveSectionModel().Search().HasQuery() || m.diffarea.ActiveSectionModel().Search().MatchesCount() == 0 {
		t.Fatalf("expected enter to show search results mode while keeping highlights")
	}
	if m.diffarea.NavMode() != diffview.NavModeLine {
		t.Fatalf("expected enter after diff search to switch to line mode")
	}
	first := m.diffarea.ActiveSectionModel().Search().Cursor()
	firstLine := m.diffarea.Unstaged.DataRef().ActiveLine

	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.diffarea.ActiveSectionModel().Search().Cursor() == first {
		t.Fatalf("expected n to move to next diff result")
	}
	if m.diffarea.Unstaged.DataRef().ActiveLine == firstLine {
		t.Fatalf("expected n to move active diff line to next match")
	}

	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N'})
	if m.diffarea.ActiveSectionModel().Search().Cursor() != first {
		t.Fatalf("expected N to move back to previous diff result")
	}

	// Moving cursor to a matched line should update the search counter cursor.
	startCursor := m.diffarea.ActiveSectionModel().Search().Cursor()
	for i := 0; i < 5; i++ {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updated.(Model)
		if m.diffarea.ActiveSectionModel().Search().Cursor() != startCursor {
			break
		}
	}
	if m.diffarea.ActiveSectionModel().Search().Cursor() == startCursor {
		t.Fatalf("expected diff cursor movement to sync search cursor when reaching a match")
	}
}

func TestSearchConfirmedSyncsToCrossPane(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	// Stage a change so the staged diff section has content containing "needle".
	testutil.WriteFile(t, repo, "README.md", "needle-one\nneedle-two\n")
	testutil.MustGitExported(t, repo, "add", "README.md")
	// Also leave an unstaged modification so both sections have diffs.
	testutil.WriteFile(t, repo, "README.md", "needle-one\nneedle-two\nneedle-three\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged

	// Activate search and type a query
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	for _, r := range []rune{'n', 'e', 'e', 'd', 'l', 'e'} {
		m = runStatusCmds(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	// Confirm with Enter
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	// Active (unstaged) section should be in results mode
	active := m.diffarea.ActiveSectionModel().Search()
	if active.Mode() != search.SearchModeResults {
		t.Fatalf("active section: expected SearchModeResults after Enter, got %v", active.Mode())
	}
	if !active.HasQuery() {
		t.Fatal("active section: expected HasQuery=true after Enter")
	}

	// Inactive (staged) section should have the query synced and be in results mode
	inactive := m.diffarea.InactiveSectionModel().Search()
	if !inactive.HasQuery() {
		t.Fatal("inactive section: expected HasQuery=true after cross-pane sync")
	}
	if inactive.Mode() != search.SearchModeResults {
		t.Fatalf("inactive section: expected SearchModeResults, got %v", inactive.Mode())
	}

	// Counter for inactive section should be non-empty (advisor check)
	counter := m.diffarea.SearchCounter(diffarea.SectionStaged, m.focus == focusDiff, m.settings.UseNerdFontIcons)
	if inactive.MatchesCount() > 0 && counter == "" {
		t.Fatal("expected non-empty counter for inactive section when it has matches")
	}
}

func TestYShowsBindingDrivenYankHint(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd != nil {
		t.Fatalf("first y should not launch command")
	}
	m = updated.(Model)
	if len(m.keys.Prefix()) == 0 || m.keys.Prefix()[0] != "y" {
		t.Fatalf("expected chord prefix=y, got %v", m.keys.Prefix())
	}

	// Chord hints are shown in the chord overlay via Manager.ChordHints().
	hints := m.keys.ChordHints()
	allDescs := ""
	for _, h := range hints {
		allDescs += " " + h.Key + " " + h.Desc
	}
	// yank-for-AI moved off the 'y' chord to 'ay'; the legacy 'ya' alias is
	// hidden, so it no longer appears in the 'y' chord hint overlay.
	for _, want := range []string{"y", "content", "l", "location", "f", "filename"} {
		if !strings.Contains(allDescs, want) {
			t.Fatalf("expected yank hint %q in ChordHints descriptions %q", want, allDescs)
		}
	}
}
