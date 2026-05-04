package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diff"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUseStackedLayoutThreshold(t *testing.T) {
	m := Model{width: 100}
	if !m.useStackedLayout() {
		t.Fatal("expected stacked layout at width 100")
	}
	m.width = 101
	if m.useStackedLayout() {
		t.Fatal("expected side-by-side layout at width 101")
	}
}

func TestSplitWidthUsesMinimumStatusPaneWidthForShortContent(t *testing.T) {
	m := Model{width: 160}
	statusW, diffW := m.splitWidth()
	if statusW != minStatusPaneWidth {
		t.Fatalf("expected minimum status width %d, got %d", minStatusPaneWidth, statusW)
	}
	if diffW != 160-minStatusPaneWidth {
		t.Fatalf("expected diff width %d, got %d", 160-minStatusPaneWidth, diffW)
	}
}

func TestSplitWidthExpandsForLongVisibleStatusRows(t *testing.T) {
	m := Model{
		width: 180,
		statusPageState: statusPageState{
			statusEntries: []statusEntry{{
				Kind:        statusEntryFile,
				DisplayName: "renamed.go",
				File: git.StageFileStatus{
					Path:         "new/path/renamed.go",
					RenameFrom:   "old/path/original-name.go",
					IndexStatus:  'R',
					WorktreeCode: ' ',
				},
				HasStaged: true,
			}},
		},
	}

	statusW, diffW := m.splitWidth()
	if statusW <= minStatusPaneWidth {
		t.Fatalf("expected status pane to grow past minimum width, got %d", statusW)
	}
	if diffW >= 180-minStatusPaneWidth {
		t.Fatalf("expected diff pane to shrink when status pane grows, got %d", diffW)
	}
	pane := ansi.Strip(m.renderStatusPane(statusW, 10))
	if !strings.Contains(pane, "old/path/original-name.go -> new/path/renamed.go") {
		t.Fatalf("expected full renamed path to fit without truncation, got:\n%s", pane)
	}
}

func TestSplitWidthHonorsMaximumStatusPaneWidth(t *testing.T) {
	m := Model{
		width: 200,
		statusPageState: statusPageState{
			branchName:    "feature/some-extremely-verbose-branch-name-that-keeps-going",
			branchBaseRef: "origin/release/very-long-train-name",
			branchSync:    git.SyncStatus{Name: git.StatusDiverged, Ahead: 12, Behind: 8},
		},
	}

	statusW, diffW := m.splitWidth()
	if statusW != maxStatusPaneWidth {
		t.Fatalf("expected status pane max width %d, got %d", maxStatusPaneWidth, statusW)
	}
	if diffW != 200-maxStatusPaneWidth {
		t.Fatalf("expected diff width %d, got %d", 200-maxStatusPaneWidth, diffW)
	}
}

func TestSplitWidthPreservesMinimumDiffWidth(t *testing.T) {
	m := Model{
		width: 101,
		statusPageState: statusPageState{
			branchName:    "feature/some-extremely-verbose-branch-name-that-keeps-going",
			branchBaseRef: "origin/release/very-long-train-name",
			branchSync:    git.SyncStatus{Name: git.StatusDiverged, Ahead: 12, Behind: 8},
		},
	}

	statusW, diffW := m.splitWidth()
	if diffW != minDiffPaneWidth {
		t.Fatalf("expected minimum diff width %d, got %d", minDiffPaneWidth, diffW)
	}
	if statusW != 101-minDiffPaneWidth {
		t.Fatalf("expected status width %d, got %d", 101-minDiffPaneWidth, statusW)
	}
}

func TestQAndEscFocusBehavior(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("expected nil cmd on esc")
	}
	m2 := updated.(Model)
	if m2.focus != focusStatus {
		t.Fatalf("esc should move focus to status")
	}

	m2.focus = focusStatus
	updated, cmd = m2.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("q in status should quit")
	}
}

func TestQAlwaysQuitsFromDiffFocus(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("expected q to quit from diff focus")
	}
}

func TestStatusLOnFileEntersDiffAndResetsSectionOnFileChange(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\n")
	testutil.WriteFile(t, repo, "b.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt", "b.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	// File a has both staged and unstaged changes.
	testutil.WriteFile(t, repo, "a.txt", "ONE\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.WriteFile(t, repo, "a.txt", "ONE-again\ntwo\n")

	// File b is only unstaged.
	testutil.WriteFile(t, repo, "b.txt", "ONE\ntwo\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus
	m.section = sectionStaged

	if len(m.statusEntries) < 2 {
		t.Fatalf("expected two status entries, got %d", len(m.statusEntries))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if m.focus != focusDiff {
		t.Fatalf("expected l on file to enter diff")
	}
	if m.section != sectionStaged {
		t.Fatalf("expected section to remain staged for same file")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if m.focus != focusStatus {
		t.Fatalf("expected h in diff to return to status")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.section != sectionUnstaged {
		t.Fatalf("expected section reset to unstaged after active file change")
	}
}

func TestStatusHFocusesParentFolder(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/ui/status")
	testutil.WriteFile(t, repo, "ui/status/model.go", "package status\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	fileIdx := -1
	for i, entry := range m.statusEntries {
		if entry.Kind == statusEntryFile && entry.Path == "ui/status/model.go" {
			fileIdx = i
			break
		}
	}
	if fileIdx < 0 {
		t.Fatalf("expected ui/status/model.go entry in status tree")
	}
	m.selected = fileIdx

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected h to schedule diff reload after focusing parent")
	}
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "ui/status" {
		t.Fatalf("expected selection to move to parent dir ui/status, got %+v", entry)
	}
}

func TestStatusHOnCompressedDirDoesNotFocusHiddenParent(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/keyboards/iris/keymaps")
	testutil.WriteFile(t, repo, "keyboards/iris/keymaps/myfile.c", "changed\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "keyboards/iris/keymaps" {
		t.Fatalf("expected compressed dir selected by default, got %+v", entry)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected no diff reload cmd when no visible parent exists")
	}
	entry, ok = m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "keyboards/iris/keymaps" {
		t.Fatalf("expected selection to stay on compressed dir, got %+v", entry)
	}
}

func TestHelpOverlayToggleAndCompactStatusBar(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 40
	m.focus = focusStatus

	line := m.helpLine()
	if !strings.Contains(line, "? help") || strings.Contains(line, "j/k") {
		t.Fatalf("expected compact status help line, got %q", line)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = updated.(Model)
	if !m.helpOpen {
		t.Fatalf("expected help overlay to open")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if !m.helpOpen {
		t.Fatalf("expected help overlay to stay open while scrolling")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.helpOpen {
		t.Fatalf("expected help overlay to close on esc")
	}
}

func TestHelpLineRightAlignsHintAndTruncatesStatus(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := New(testutil.TempRepo(t))
	m.ready = true
	m.width = 48
	m.focus = focusStatus
	m.statusMsg = "this is a very long status message that should truncate"

	line := m.helpLine()
	plain := ansi.Strip(line)

	if ansi.StringWidth(plain) != m.width {
		t.Fatalf("expected footer width %d, got %d (%q)", m.width, ansi.StringWidth(plain), plain)
	}
	if !strings.Contains(plain, "...") {
		t.Fatalf("expected truncated status with ellipsis, got %q", plain)
	}
	if !strings.HasSuffix(plain, "· 󰉸 context: 1 · status · ? help") {
		t.Fatalf("expected hint right-aligned at end, got %q", plain)
	}
}

func TestStatusPaneShowsBranchSummaryInTitle(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := New(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.branchName = "feature/test"
	m.branchBaseRef = "origin/main"
	m.branchSync = git.SyncStatus{Name: git.StatusAhead, Ahead: 2}

	pane := ansi.Strip(m.renderStatusPane(72, 10))
	if !strings.Contains(pane, "Status (") {
		t.Fatalf("expected branch summary in status title, got:\n%s", pane)
	}
	if !strings.Contains(pane, "feature/test") {
		t.Fatalf("expected branch summary to include branch name, got:\n%s", pane)
	}
	if !strings.Contains(pane, "↑2") {
		t.Fatalf("expected branch summary to include ahead state, got:\n%s", pane)
	}
	if strings.Contains(pane, "vs origin/main") {
		t.Fatalf("expected default base ref to stay hidden, got:\n%s", pane)
	}
}

func TestNewWithInitialPathSelectsFileAndKeepsStatusFocus(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/dir")
	testutil.WriteFile(t, repo, "dir/a.txt", "one\n")
	testutil.WriteFile(t, repo, "dir/b.txt", "two\n")

	m := NewWithSettings(repo, Settings{
		DiffContextLines: 1,
		UseNerdFontIcons: true,
		InitialPath:      "dir/b.txt",
	})

	entry, ok := m.selectedStatusEntry()
	if !ok {
		t.Fatal("expected selected entry")
	}
	if entry.Kind != statusEntryFile || entry.Path != "dir/b.txt" {
		t.Fatalf("selected entry = %+v, want file dir/b.txt", entry)
	}
	if m.focus != focusStatus {
		t.Fatalf("focus = %v, want %v", m.focus, focusStatus)
	}
	if m.activeFilePath != "dir/b.txt" {
		t.Fatalf("activeFilePath = %q, want %q", m.activeFilePath, "dir/b.txt")
	}
}

func TestBranchSummaryTitleShowsBaseOnlyWhenNonDefault(t *testing.T) {
	m := Model{
		settings: Settings{UseNerdFontIcons: true},
		statusPageState: statusPageState{
			branchName:    "feature/x",
			branchBaseRef: "origin/release",
			branchSync:    git.SyncStatus{Name: git.StatusBehind, Behind: 1},
		},
	}
	line := m.branchSummaryTitleSuffix()
	if !strings.Contains(line, " feature/x ↓1") {
		t.Fatalf("unexpected title suffix: %q", line)
	}
	if !strings.Contains(line, "vs origin/release") {
		t.Fatalf("expected non-default base ref in title suffix: %q", line)
	}
}

func TestReloadBranchStateUsesBranchUpstream(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")
	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature")
	testutil.WriteFile(t, wtDir, "ahead.txt", "ahead")
	testutil.CommitAll(t, wtDir, "ahead")

	m := New(wtDir)
	m.reloadBranchState()

	if m.branchName != "feature" {
		t.Fatalf("expected current branch feature, got %q", m.branchName)
	}
	if m.branchBaseRef != "origin/feature" {
		t.Fatalf("expected upstream base ref origin/feature, got %q", m.branchBaseRef)
	}

	// Branch sync is now async; run the cmd synchronously to get the result.
	if cmd := m.cmdLoadBranchSync(); cmd != nil {
		updated, _ := m.Update(cmd())
		m = updated.(Model)
	}
	if m.branchSync.Name != git.StatusAhead || m.branchSync.Ahead != 1 {
		t.Fatalf("expected branch sync ahead of origin/feature, got %+v", m.branchSync)
	}
}

func TestHelpLineShowsVisualAtLeftInDiffFocus(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := New(testutil.TempRepo(t))
	m.ready = true
	m.width = 96
	m.focus = focusDiff
	m.navMode = navLine
	m.unstaged.visualActive = true

	line := m.helpLine()
	plain := ansi.Strip(line)

	if !strings.HasPrefix(plain, "VISUAL") {
		t.Fatalf("expected VISUAL indicator at start of footer, got %q", plain)
	}
	if !strings.HasSuffix(plain, "· 󰉸 context: 1 · diff: mode:line · render:unified · wrap:on · ? help") {
		t.Fatalf("expected diff hint at end of footer, got %q", plain)
	}
}

func TestToggleSideBySideModeWithS(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "a.txt", "two\n")

	m := New(repo)
	m.ready = true
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	updated, colorizeCmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.renderMode != renderSideBySide {
		t.Fatalf("expected render mode side-by-side, got %v", m.renderMode)
	}
	if !strings.Contains(m.statusMsg, "side-by-side mode") {
		t.Fatalf("expected side-by-side status message, got %q", m.statusMsg)
	}
	// Delta colorization is async; run the cmd synchronously to get colored output.
	if colorizeCmd != nil {
		updated, _ = m.Update(colorizeCmd())
		m = updated.(Model)
	}
	view := ansi.Strip(m.renderDiffPane(80, 16))
	if strings.Contains(view, "No file selected") {
		t.Fatalf("expected side-by-side diff content, got:\n%s", view)
	}
	if !strings.Contains(view, "▌ ") {
		t.Fatalf("expected active hunk indicator in side-by-side view, got:\n%s", view)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.renderMode != renderUnified {
		t.Fatalf("expected render mode unified, got %v", m.renderMode)
	}
}

func TestToggleSideBySideModeWithSFromStatusPane(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "status-s.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "status-s.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "status-s.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.renderMode != renderSideBySide {
		t.Fatalf("expected render mode side-by-side from status pane, got %v", m.renderMode)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.renderMode != renderUnified {
		t.Fatalf("expected render mode unified after second toggle, got %v", m.renderMode)
	}
}

func TestStripUnifiedVisibleMarkerRemovesChangedPrefix(t *testing.T) {
	line := "  1 ⋮    │-old"
	got := diff.StripUnifiedVisibleMarker(line, '-')
	if strings.Contains(got, "│-old") {
		t.Fatalf("expected visible marker stripped, got %q", got)
	}
	if !strings.Contains(got, "│ old") {
		t.Fatalf("expected content alignment preserved, got %q", got)
	}

	raw := "+new"
	got = diff.StripUnifiedVisibleMarker(raw, '+')
	if got != " new" {
		t.Fatalf("expected raw unified marker replaced with space, got %q", got)
	}

	line = "    ⋮ 579│+\tline := \"  1 ⋮    │-old\""
	got = diff.StripUnifiedVisibleMarker(line, '+')
	if strings.Contains(got, "│+\tline") {
		t.Fatalf("expected gutter marker stripped before source text, got %q", got)
	}
}

func TestAdjustDiffContextLinesInDiffFocus(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "ctx.txt", "one\ntwo\nthree\n")
	testutil.MustGitExported(t, repo, "add", "ctx.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "ctx.txt", "zero\none\ntwo\nTHREE\n")

	m := NewWithSettings(repo, Settings{DiffContextLines: 1, UseNerdFontIcons: true})
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.currentDiffContextLines() != 1 {
		t.Fatalf("expected initial diff context 1, got %d", m.currentDiffContextLines())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	if m.currentDiffContextLines() != 2 {
		t.Fatalf("expected diff context 2 after ], got %d", m.currentDiffContextLines())
	}
	if !strings.Contains(m.statusMsg, "diff context: 2") {
		t.Fatalf("expected diff context status message, got %q", m.statusMsg)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	if m.currentDiffContextLines() != 1 {
		t.Fatalf("expected diff context 1 after [, got %d", m.currentDiffContextLines())
	}
}

func TestUnifiedDiffViewHidesVisibleChangeMarkers(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "clean.txt", "old-value\nkeep\n")
	testutil.MustGitExported(t, repo, "add", "clean.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "clean.txt", "new-value\nkeep\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 20

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	view := ansi.Strip(m.renderDiffPane(100, 12))
	if strings.Contains(view, "+new-value") || strings.Contains(view, "-old-value") {
		t.Fatalf("expected unified view to hide visible +/- markers, got:\n%s", view)
	}
	if !strings.Contains(view, "new-value") || !strings.Contains(view, "old-value") {
		t.Fatalf("expected unified view to keep changed content visible, got:\n%s", view)
	}

	stagedBefore, err := git.DiffPath(repo, "clean.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff before apply: %v", err)
	}
	if stagedBefore != "" {
		t.Fatalf("expected no staged diff before apply, got:\n%s", stagedBefore)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	stagedAfter, err := git.DiffPath(repo, "clean.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff after apply: %v", err)
	}
	if !strings.Contains(stagedAfter, "old-value") && !strings.Contains(stagedAfter, "new-value") {
		t.Fatalf("expected staging to keep using raw unified diff mappings, got:\n%s", stagedAfter)
	}
}

func TestDiffBodyPaddingStylesChangedRows(t *testing.T) {
	added := diff.DiffBodyPadding(diff.RowAdded, 3)
	removed := diff.DiffBodyPadding(diff.RowRemoved, 3)
	plain := diff.DiffBodyPadding(diff.RowPlain, 3)

	if ansi.Strip(added) != "   " || ansi.Strip(removed) != "   " || plain != "   " {
		t.Fatalf("expected padding width preserved")
	}
	if added == "   " || removed == "   " {
		t.Fatalf("expected changed row padding to carry styling")
	}
}

func TestAdjustDiffContextLinesIsSessionOnly(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "ctx-status.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "ctx-status.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "ctx-status.txt", "ONE\ntwo\n")

	m := NewWithSettings(repo, Settings{DiffContextLines: 3, UseNerdFontIcons: true})
	m.ready = true
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	if m.currentDiffContextLines() != 2 {
		t.Fatalf("expected session diff context 2 after [, got %d", m.currentDiffContextLines())
	}
	if m.settings.DiffContextLines != 3 {
		t.Fatalf("expected settings diff context to remain 3, got %d", m.settings.DiffContextLines)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	if m.currentDiffContextLines() != 3 {
		t.Fatalf("expected session diff context 3 after ], got %d", m.currentDiffContextLines())
	}
}

func TestSideBySideModeAllowsHunkStaging(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "b.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "b.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := New(repo)
	m.ready = true
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	staged, err := git.DiffPath(repo, "b.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if !strings.Contains(staged, "+two") {
		t.Fatalf("expected staged diff in side-by-side hunk mode, got:\n%s", staged)
	}
}

func TestSideBySideModeAllowsLineModeToggle(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "c.txt", "one\ntwo\n")

	m := New(repo)
	m.ready = true
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if m.navMode != navLine {
		t.Fatalf("expected nav mode to switch to line in side-by-side, got %v", m.navMode)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	staged, err := git.DiffPath(repo, "c.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if !strings.Contains(staged, "+one") {
		t.Fatalf("expected staged line diff in side-by-side line mode, got:\n%s", staged)
	}
}

func TestSideBySideModeAllowsVisualLineRangeStaging(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "v.txt", "new-1\nnew-2\nnew-3\n")

	m := New(repo)
	m.ready = true
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := git.DiffPath(repo, "v.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if !strings.Contains(staged, "+new-1") || !strings.Contains(staged, "+new-2") {
		t.Fatalf("expected visual range lines staged in side-by-side mode, got:\n%s", staged)
	}
}

func TestRefreshesOnFocusMsg(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")

	m := New(repo)
	m.ready = true

	countBefore := len(m.statusEntries)
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	updated, _ := m.Update(tea.FocusMsg{})
	m = updated.(Model)

	if len(m.statusEntries) <= countBefore {
		t.Fatalf("expected refresh on focus to include new file; before=%d after=%d", countBefore, len(m.statusEntries))
	}
}

func TestFocusMsgRefreshPreservesDiffScrollOffset(t *testing.T) {
	repo := testutil.TempRepo(t)
	base := make([]string, 0, 80)
	updated := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		base = append(base, fmt.Sprintf("old-%03d", i))
		updated = append(updated, fmt.Sprintf("new-%03d", i))
	}
	testutil.WriteFile(t, repo, "scroll-focus.txt", strings.Join(base, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "scroll-focus.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "scroll-focus.txt", strings.Join(updated, "\n")+"\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 28
	m.focus = focusDiff
	m.syncDiffViewports()
	m.unstaged.viewport.SetYOffset(9)

	updatedModel, _ := m.Update(tea.FocusMsg{})
	m = updatedModel.(Model)

	if got := m.unstaged.viewport.YOffset(); got != 9 {
		t.Fatalf("expected focus refresh to preserve diff scroll offset, got %d", got)
	}
}

func TestViewEnablesReportFocus(t *testing.T) {
	m := New(testutil.TempRepo(t))
	m.ready = true
	v := m.View()
	if !v.ReportFocus {
		t.Fatalf("expected ReportFocus enabled on stage view")
	}
}

func TestFullscreenDiffHidesStatusPane(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 30
	m.focus = focusDiff
	m.diffFullscreen = true

	v := m.View()
	plain := ansi.Strip(v.Content)
	if strings.Contains(plain, "Status") {
		t.Fatalf("expected status pane hidden in fullscreen diff view")
	}
}

func TestSpaceStagesSingleLineInLineMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "line-1\nline-2\n")

	m := New(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	m.applySelection()

	staged, err := git.DiffPath(repo, "line.txt", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if staged == "" {
		t.Fatalf("expected staged diff after line-mode space, status=%q", m.statusMsg)
	}
}

func TestStatusSpaceTogglesWholeFile(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := git.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if staged == "" {
		t.Fatalf("expected file to be staged by status space")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err = git.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached after unstage: %v", err)
	}
	if staged != "" {
		t.Fatalf("expected file to be unstaged by second status space")
	}
}

func TestStatusDDiscardsUntrackedFileAfterConfirm(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "new.txt", "new\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if !m.confirmOpen {
		t.Fatalf("expected discard confirmation to open")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	if _, err := os.Stat(repo + "/new.txt"); err == nil {
		t.Fatalf("expected untracked file to be deleted after discard confirm")
	}
	if len(m.statusEntries) != 0 {
		t.Fatalf("expected no status entries after discard, got %d", len(m.statusEntries))
	}
}

func TestDiffUnstagedDDiscardsLineAfterConfirm(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "line.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "line.txt", "ONE\ntwo\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if !m.confirmOpen {
		t.Fatalf("expected discard confirmation in unstaged diff")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	unstaged, err := git.DiffPath(repo, "line.txt", false, 1)
	if err != nil {
		t.Fatalf("DiffPath unstaged: %v", err)
	}
	if strings.Contains(unstaged, "+ONE") || !strings.Contains(unstaged, "-one") {
		t.Fatalf("expected selected +line to be discarded from worktree, got:\n%s", unstaged)
	}
}

func TestDiffStagedDUnstagesSelectionWithoutConfirm(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "one\ntwo\n")

	m := New(repo)
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	m.focus = focusDiff
	m.section = sectionStaged
	m.navMode = navLine

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)

	if m.confirmOpen {
		t.Fatalf("did not expect discard confirm in staged diff; d should unstage")
	}
	unstaged, err := git.DiffPath(repo, "line.txt", false, 1)
	if err != nil {
		t.Fatalf("DiffPath unstaged: %v", err)
	}
	if strings.TrimSpace(unstaged) == "" {
		t.Fatalf("expected unstaged diff after using d in staged view")
	}
}

func TestYankFilenameWithYFInStatusView(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)

	if got != "a.txt" {
		t.Fatalf("expected yanked filename, got %q", got)
	}
}

func TestYankLocationWithYLInStatusViewYanksFilenameOnly(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "b.txt", "one\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)

	if got != "@b.txt" {
		t.Fatalf("expected yl in status to yank filename only, got %q", got)
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

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

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

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

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

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

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

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

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

func TestDiffJKScrollsWithoutMovingCursor(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "# test\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	beforeLine := m.unstaged.activeLine
	beforeOffset := m.unstaged.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	m = updated.(Model)
	if m.unstaged.activeLine != beforeLine {
		t.Fatalf("J changed active line: got %d want %d", m.unstaged.activeLine, beforeLine)
	}
	maxOffset := m.unstaged.viewport.TotalLineCount() - m.unstaged.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	expectedDelta := 3
	if beforeOffset+expectedDelta > maxOffset {
		expectedDelta = maxOffset - beforeOffset
	}
	if expectedDelta < 0 {
		expectedDelta = 0
	}
	if got := m.unstaged.viewport.YOffset(); got != beforeOffset+expectedDelta {
		t.Fatalf("J should scroll by up to 3: before=%d after=%d expected=%d", beforeOffset, got, beforeOffset+expectedDelta)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	m = updated.(Model)
	if m.unstaged.activeLine != beforeLine {
		t.Fatalf("K changed active line: got %d want %d", m.unstaged.activeLine, beforeLine)
	}
	if expectedDelta > 0 {
		if got := m.unstaged.viewport.YOffset(); got >= beforeOffset+expectedDelta {
			t.Fatalf("K should scroll up on first press: offset after K=%d", got)
		}
	}
}

func TestHunkModeJKScrollsLargeHunkBeforeSwitching(t *testing.T) {
	repo := testutil.TempRepo(t)

	base := make([]string, 0, 48)
	for i := 1; i <= 48; i++ {
		base = append(base, fmt.Sprintf("line-%02d", i))
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(base, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "big.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := append([]string{}, base...)
	for i := 0; i < 20; i++ {
		updated[i] = "new-" + updated[i]
	}
	for i := 34; i < 38; i++ {
		updated[i] = "new-" + updated[i]
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(updated, "\n")+"\n")

	m := New(repo)
	m.ready = true
	m.width = 100
	m.height = 16
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navHunk
	m.syncDiffViewports()

	if m.unstaged.activeHunk != 0 {
		t.Fatalf("expected first hunk active initially, got %d", m.unstaged.activeHunk)
	}
	beforeOffset := m.unstaged.viewport.YOffset()

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updatedModel.(Model)
	if m.unstaged.activeHunk != 0 {
		t.Fatalf("expected j to scroll large hunk before switching, activeHunk=%d", m.unstaged.activeHunk)
	}
	if m.unstaged.viewport.YOffset() <= beforeOffset {
		t.Fatalf("expected j to scroll down within large hunk")
	}

	midOffset := m.unstaged.viewport.YOffset()
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = updatedModel.(Model)
	if m.unstaged.activeHunk != 0 {
		t.Fatalf("expected k to scroll large hunk before switching, activeHunk=%d", m.unstaged.activeHunk)
	}
	if m.unstaged.viewport.YOffset() >= midOffset {
		t.Fatalf("expected k to scroll up within large hunk")
	}
}

func TestHunkOverflowViewportMarkers(t *testing.T) {
	repo := testutil.TempRepo(t)

	base := make([]string, 0, 40)
	for i := 1; i <= 40; i++ {
		base = append(base, fmt.Sprintf("line-%02d", i))
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(base, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "big.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := append([]string{}, base...)
	for i := 0; i < 24; i++ {
		updated[i] = "new-" + updated[i]
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(updated, "\n")+"\n")

	assertMarkers := func(useNerd bool, up, down string) {
		t.Helper()
		m := NewWithSettings(repo, Settings{DiffContextLines: 1, UseNerdFontIcons: useNerd})
		m.ready = true
		m.width = 100
		m.height = 16
		m.focus = focusDiff
		m.section = sectionUnstaged
		m.navMode = navHunk
		m.syncDiffViewports()

		m.unstaged.viewport.SetYOffset(3)
		pane := m.renderSectionPane(80, 10, "Unstaged", &m.unstaged, sectionUnstaged)

		if strings.Contains(pane, "hunk>view") {
			t.Fatalf("unexpected legacy hunk>view indicator in pane:\n%s", pane)
		}
		if !strings.Contains(pane, up) || !strings.Contains(pane, down) {
			t.Fatalf("expected overflow markers %q and %q in pane:\n%s", up, down, pane)
		}
	}

	assertMarkers(true, "", "")
	assertMarkers(false, "↑", "↓")
}

func TestGInStatusAndDiffJumpsBottom(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")
	testutil.WriteFile(t, repo, "c.txt", "three\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus
	m.selected = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.selected != len(m.statusEntries)-1 {
		t.Fatalf("expected G to jump status selection to bottom, got %d", m.selected)
	}

	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine
	if len(m.unstaged.parsed.Changed) == 0 {
		t.Fatalf("expected unstaged changes in diff view")
	}
	m.unstaged.activeLine = 0

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.unstaged.activeLine != len(m.unstaged.parsed.Changed)-1 {
		t.Fatalf("expected G to jump active diff line to bottom, got %d", m.unstaged.activeLine)
	}
}

func TestUppercaseGUsingShiftedCodeJumpsBottom(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus
	m.selected = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "G", ShiftedCode: 'G'})
	m = updated.(Model)
	if m.selected != len(m.statusEntries)-1 {
		t.Fatalf("expected shifted G to jump to bottom, got %d", m.selected)
	}
}

func TestUppercaseGUsingShiftModifierJumpsBottom(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus
	m.selected = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g", Mod: tea.ModShift})
	m = updated.(Model)
	if m.selected != len(m.statusEntries)-1 {
		t.Fatalf("expected shifted modifier G to jump to bottom, got %d", m.selected)
	}
}

func TestGInDiffHunkModeJumpsViewportToBottom(t *testing.T) {
	repo := testutil.TempRepo(t)
	base := make([]string, 0, 40)
	for i := 1; i <= 40; i++ {
		base = append(base, fmt.Sprintf("line-%02d", i))
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(base, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "big.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := append([]string{}, base...)
	for i := 0; i < 28; i++ {
		updated[i] = "new-" + updated[i]
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(updated, "\n")+"\n")

	m := New(repo)
	m.ready = true
	m.width = 100
	m.height = 16
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navHunk
	m.syncDiffViewports()

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "G", ShiftedCode: 'G'})
	m = updatedModel.(Model)

	maxOffset := m.unstaged.viewport.TotalLineCount() - m.unstaged.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if got := m.unstaged.viewport.YOffset(); got != maxOffset {
		t.Fatalf("expected G to jump diff viewport to bottom, got %d want %d", got, maxOffset)
	}
}

func TestCtrlDAndCtrlUScrollStatusAndDiff(t *testing.T) {
	repo := testutil.TempRepo(t)
	for i := 0; i < 16; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("f%02d.txt", i), "x\n")
	}

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.focus = focusStatus
	beforeSel := m.selected

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.selected <= beforeSel {
		t.Fatalf("expected ctrl+d to move status selection down")
	}

	midSel := m.selected
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.selected >= midSel {
		t.Fatalf("expected ctrl+u to move status selection up")
	}

	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navHunk
	m.syncDiffViewports()
	beforeOffset := m.unstaged.viewport.YOffset()

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.unstaged.viewport.YOffset() < beforeOffset {
		t.Fatalf("expected ctrl+d to scroll diff down")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.unstaged.viewport.YOffset() > beforeOffset {
		t.Fatalf("expected ctrl+u to scroll diff up")
	}
}

func TestStatusFileIconDeletedAndFallback(t *testing.T) {
	deleted := git.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'}

	nerd := statusPaneIconsFor(true)
	if got := statusFileIcon(deleted, false, nerd); got != "" {
		t.Fatalf("expected deleted nerd icon, got %q", got)
	}

	plain := statusPaneIconsFor(false)
	if got := statusFileIcon(deleted, false, plain); got != "D" {
		t.Fatalf("expected deleted fallback icon, got %q", got)
	}
}

func TestStatusEntryColorDeletedFileIsDim(t *testing.T) {
	entry := statusEntry{
		Kind: statusEntryFile,
		File: git.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'},
	}
	if got := statusEntryColor(entry); got != "#a6adc8" {
		t.Fatalf("expected dim deleted color, got %q", got)
	}
}

func TestStatusFileIconRenamedAndFallback(t *testing.T) {
	renamed := git.StageFileStatus{Path: "new.txt", RenameFrom: "old.txt", IndexStatus: 'R'}

	nerd := statusPaneIconsFor(true)
	if got := statusFileIcon(renamed, false, nerd); got != "󰁔" {
		t.Fatalf("expected renamed nerd icon, got %q", got)
	}

	plain := statusPaneIconsFor(false)
	if got := statusFileIcon(renamed, false, plain); got != "R" {
		t.Fatalf("expected renamed fallback icon, got %q", got)
	}
}

func TestStatusEntryColorRenamedFileIsBlue(t *testing.T) {
	entry := statusEntry{
		Kind: statusEntryFile,
		File: git.StageFileStatus{Path: "new.txt", RenameFrom: "old.txt", IndexStatus: 'R'},
	}
	if got := statusEntryColor(entry); got != "#89b4fa" {
		t.Fatalf("expected renamed color, got %q", got)
	}
}

func TestStatusMessageClearsAfterTimeoutTick(t *testing.T) {
	m := New(testutil.TempRepo(t))
	m.ready = true
	m.statusMsg = "temporary"
	m.statusUntil = time.Now().Add(-time.Second)

	updated, _ := m.Update(statusTickMsg{})
	m = updated.(Model)
	if m.statusMsg != "" {
		t.Fatalf("expected status message to clear after timeout tick")
	}
}

func TestStatusSelectionDebouncesDiffReload(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	before := m.activeFilePath
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected j in status to schedule debounced reload")
	}
	if m.activeFilePath != before {
		t.Fatalf("expected active file to remain unchanged before debounce fires")
	}

	updated, _ = m.Update(diffReloadMsg{seq: m.diffReloadSeq})
	m = updated.(Model)
	if m.activeFilePath == before {
		t.Fatalf("expected active file to update after debounce message")
	}
}

func TestStageSearchStatusModeAndNavigation(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "apple.txt", "one\n")
	testutil.WriteFile(t, repo, "apricot.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.searchMode != searchModeInput {
		t.Fatalf("expected search input mode after /")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(Model)
	if len(m.searchMatches) < 2 {
		t.Fatalf("expected multiple status search matches, got %d", len(m.searchMatches))
	}

	first := m.selected
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.searchMode != searchModeNone || m.searchQuery == "" || len(m.searchMatches) == 0 {
		t.Fatalf("expected enter to return to normal mode while keeping search highlights")
	}
	if line := ansi.Strip(m.helpLine()); !strings.Contains(line, "1/2") {
		t.Fatalf("expected persistent search counter in footer, got %q", line)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.selected == first {
		t.Fatalf("expected n to move to next search result")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.searchMode != searchModeNone || m.searchQuery != "" || len(m.searchMatches) != 0 {
		t.Fatalf("expected esc to clear search state")
	}
}

func TestStageSearchModeShowsOverlay(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "apple.txt", "one\n")

	m := New(repo)
	m.ready = true
	m.width = 60
	m.height = 20
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	overlay := ansi.Strip(m.searchInputOverlayView())
	if !strings.Contains(overlay, "Search") {
		t.Fatalf("expected overlay to contain 'Search', got %q", overlay)
	}
}

func TestStageSearchDiffModeAndPrevNextKeys(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "needle-one\nline\nneedle-two\n")

	m := New(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.section = sectionUnstaged

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.searchMode != searchModeInput {
		t.Fatalf("expected diff search input mode after /")
	}

	for _, r := range []rune{'n', 'e', 'e', 'd', 'l', 'e'} {
		updated, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}
	if len(m.searchMatches) < 2 {
		t.Fatalf("expected multiple diff search matches, got %d", len(m.searchMatches))
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.searchMode != searchModeNone || m.searchQuery == "" || len(m.searchMatches) == 0 {
		t.Fatalf("expected enter to return to normal mode while keeping search highlights")
	}
	if m.navMode != navLine {
		t.Fatalf("expected enter after diff search to switch to line mode")
	}
	first := m.searchCursor
	firstLine := m.unstaged.activeLine

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.searchCursor == first {
		t.Fatalf("expected n to move to next diff result")
	}
	if m.unstaged.activeLine == firstLine {
		t.Fatalf("expected n to move active diff line to next match")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N'})
	m = updated.(Model)
	if m.searchCursor != first {
		t.Fatalf("expected N to move back to previous diff result")
	}

	// Moving cursor to a matched line should update the search counter cursor.
	startCursor := m.searchCursor
	for i := 0; i < 5; i++ {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updated.(Model)
		if m.searchCursor != startCursor {
			break
		}
	}
	if m.searchCursor == startCursor {
		t.Fatalf("expected diff cursor movement to sync search cursor when reaching a match")
	}
}

func TestStageSearchDiffUsesRightEdgeIndicatorInHunkMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "needle-one\nline\nneedle-two\n")

	m := New(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navHunk
	m.searchQuery = "needle"
	m.searchScope = searchScopeUnstaged
	m.recomputeSearchMatches()

	pane := m.renderSectionPane(80, 12, "Unstaged", &m.unstaged, sectionUnstaged)
	if !strings.Contains(ansi.Strip(pane), "needle") {
		t.Fatalf("expected search match text highlighted in diff pane")
	}
}

func TestDiffJDoesNotOverscrollPastContent(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "# test\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.section = sectionUnstaged

	for i := 0; i < 300; i++ {
		updated, _ := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
		m = updated.(Model)
	}

	maxOffset := m.unstaged.viewport.TotalLineCount() - m.unstaged.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if got := m.unstaged.viewport.YOffset(); got != maxOffset {
		t.Fatalf("overscrolled: got offset=%d want=%d", got, maxOffset)
	}
}

func TestApplySelection_DoesNotSwitchSectionWhenHunksRemain(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "l01\nl02\nl03\nl04\nl05\nl06\nl07\nl08\nl09\nl10\nl11\nl12\nl13\nl14\nl15\nl16\nl17\nl18\nl19\nl20\nl21\nl22\nl23\nl24\nl25\nl26\nl27\nl28\nl29\nl30\n")
	testutil.CommitAll(t, repo, "baseline")
	testutil.WriteFile(t, repo, "README.md", "L01\nl02\nl03\nl04\nl05\nl06\nl07\nl08\nl09\nl10\nl11\nl12\nl13\nl14\nl15\nl16\nl17\nl18\nl19\nL20\nl21\nl22\nl23\nl24\nl25\nl26\nl27\nl28\nl29\nl30\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.syncDiffViewports()
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navHunk
	m.unstaged.activeHunk = 0
	if len(m.unstaged.parsed.Hunks) < 2 {
		t.Fatalf("expected at least 2 hunks before apply, got %d", len(m.unstaged.parsed.Hunks))
	}

	cmd := m.applySelection()
	if cmd != nil {
		// animation may or may not be set; ignore command
	}
	if m.section != sectionUnstaged {
		t.Fatalf("section switched unexpectedly while hunks remain: got=%v", m.section)
	}
	if len(m.unstaged.parsed.Hunks) == 0 {
		t.Fatalf("expected unstaged hunks to remain after staging first hunk")
	}
}

func TestCCTriggersCommitCommand(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd != nil {
		t.Fatalf("first c should not launch command")
	}
	m = updated.(Model)
	if m.keyPrefix != "c" {
		t.Fatalf("expected keyPrefix=c after first c, got %q", m.keyPrefix)
	}
	if hint := ansi.Strip(m.statusMsg); !strings.Contains(hint, "cc") || !strings.Contains(hint, "git commit") {
		t.Fatalf("expected binding-driven commit hint, got %q", hint)
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatalf("second c should launch commit command")
	}
	m = updated.(Model)
	if m.keyPrefix != "" {
		t.Fatalf("expected keyPrefix reset after cc, got %q", m.keyPrefix)
	}
}

func TestYShowsBindingDrivenYankHint(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := New(repo)
	m.ready = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd != nil {
		t.Fatalf("first y should not launch command")
	}
	m = updated.(Model)

	hint := ansi.Strip(m.statusMsg)
	for _, want := range []string{"yy", "content", "yl", "location", "ya", "all", "yf", "filename"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("expected yank hint %q in %q", want, hint)
		}
	}
}

func TestGGJumpsToTop(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := New(repo)
	m.ready = true
	m.focus = focusStatus
	m.statusEntries = []statusEntry{{Kind: statusEntryFile}, {Kind: statusEntryFile}, {Kind: statusEntryFile}}
	m.selected = 2

	// First g sets keyPrefix
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if m.keyPrefix != "g" {
		t.Fatalf("expected keyPrefix=g after first g, got %q", m.keyPrefix)
	}

	// Second g jumps to top
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd == nil {
		t.Fatalf("gg should schedule a diff reload after jumping to top")
	}
	m = updated.(Model)
	if m.selected != 0 {
		t.Fatalf("expected gg to jump to top, got selected=%d", m.selected)
	}
}

func TestLTriggersLazygitLogCommand(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := New(repo)
	m.ready = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'L', Text: "L", ShiftedCode: 'L', Mod: tea.ModShift})
	if cmd == nil {
		t.Fatalf("L should launch lazygit log command")
	}
	m = updated.(Model)
	if m.statusMsg == "" {
		t.Fatalf("expected status message after L")
	}
}

func TestGLNavigatesToLogWhenNavigationEnabled(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := NewWithSettings(repo, Settings{EnableNavigation: true})
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil {
		t.Fatalf("gl should navigate to log when navigation is enabled")
	}
	route, ok := nav.IsReplace(cmd())
	if !ok {
		t.Fatalf("expected nav replace message")
	}
	if route.Kind != nav.RouteLog {
		t.Fatalf("expected log route, got %q", route.Kind)
	}
	m = updated.(Model)
	if m.keyPrefix != "" {
		t.Fatalf("expected keyPrefix reset after gl, got %q", m.keyPrefix)
	}
}

func TestNavigationStartupDefersInitialDiffLoad(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "dirty.txt", "dirty\n")

	m := NewWithSettings(repo, Settings{EnableNavigation: true})
	if m.activeFilePath != "" {
		t.Fatalf("expected initial navigation startup to skip diff load, got %q", m.activeFilePath)
	}
	if len(m.unstaged.parsed.Hunks) != 0 {
		t.Fatalf("expected no diff hunks before startup load, got %d", len(m.unstaged.parsed.Hunks))
	}

	updated, _ := m.Update(statusStartupLoadMsg{})
	m = updated.(Model)
	if m.activeFilePath != "dirty.txt" {
		t.Fatalf("expected startup load to select dirty diff, got %q", m.activeFilePath)
	}
	if len(m.unstaged.parsed.Hunks) == 0 {
		t.Fatalf("expected diff hunks after startup load")
	}
}

func TestGOOpensOutputModal(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := New(repo)
	m.ready = true
	m.outputContent = "hello"

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd != nil {
		t.Fatalf("first g should not launch command")
	}
	m = updated.(Model)
	if m.keyPrefix != "g" {
		t.Fatalf("expected keyPrefix=g after first g, got %q", m.keyPrefix)
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		t.Fatalf("go should not launch a command")
	}
	m = updated.(Model)
	if !m.outputOpen {
		t.Fatalf("expected go to open output modal")
	}
}

func TestEOpensEditorFromStatusAndDiff(t *testing.T) {
	t.Setenv("EDITOR", "true")
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "edit.txt", "one\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected e in status view to launch editor command")
	}

	m.focus = focusDiff
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected e in diff view to launch editor command")
	}
}

func TestEditorLaunchArgsUsesGotoForKnownEditors(t *testing.T) {
	tests := []struct {
		name   string
		editor string
		want   string
	}{
		{name: "code", editor: "code", want: "--goto /tmp/x.go:12"},
		{name: "vim", editor: "nvim", want: "+12 /tmp/x.go"},
		{name: "sublime", editor: "subl", want: "/tmp/x.go:12"},
		{name: "fallback", editor: "emacs", want: "/tmp/x.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(editorLaunchArgs(tt.editor, nil, "/tmp/x.go", 12), " ")
			if !strings.Contains(got, tt.want) {
				t.Fatalf("editorLaunchArgs(%q)=%q, want to contain %q", tt.editor, got, tt.want)
			}
		})
	}
}

func TestEditorLineForCurrentSelectionInDiffMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "old-1\nold-2\nold-3\n")
	testutil.MustGitExported(t, repo, "add", "line.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "line.txt", "new-1\nnew-2\nnew-3\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	for i, cl := range m.unstaged.parsed.Changed {
		if cl.NewLine == 2 {
			m.unstaged.activeLine = i
			break
		}
	}

	line := m.editorLineForCurrentSelection()
	if line != 2 {
		t.Fatalf("editorLineForCurrentSelection()=%d, want 2", line)
	}
}

func TestMouseWheelScrollsUnstagedDiffViewport(t *testing.T) {
	repo := testutil.TempRepo(t)
	before := make([]string, 0, 80)
	after := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		before = append(before, fmt.Sprintf("old-%03d", i))
		after = append(after, fmt.Sprintf("new-%03d", i))
	}
	testutil.WriteFile(t, repo, "scroll.txt", strings.Join(before, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "scroll.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "scroll.txt", strings.Join(after, "\n")+"\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 26
	m.syncDiffViewports()
	m.focus = focusDiff

	beforeOffset := m.unstaged.viewport.YOffset()
	updated, _ := m.Update(tea.MouseWheelMsg{X: 50, Y: 6, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.unstaged.viewport.YOffset() <= beforeOffset {
		t.Fatalf("expected unstaged viewport to scroll down, before=%d after=%d", beforeOffset, m.unstaged.viewport.YOffset())
	}
}

func TestMouseWheelScrollsStagedDiffViewport(t *testing.T) {
	repo := testutil.TempRepo(t)
	before := make([]string, 0, 80)
	after := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		before = append(before, fmt.Sprintf("old-%03d", i))
		after = append(after, fmt.Sprintf("new-%03d", i))
	}
	testutil.WriteFile(t, repo, "staged-scroll.txt", strings.Join(before, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "staged-scroll.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "staged-scroll.txt", strings.Join(after, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "staged-scroll.txt")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 26
	m.syncDiffViewports()
	m.focus = focusDiff

	beforeOffset := m.staged.viewport.YOffset()
	updated, _ := m.Update(tea.MouseWheelMsg{X: 50, Y: 6, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.staged.viewport.YOffset() <= beforeOffset {
		t.Fatalf("expected staged viewport to scroll down, before=%d after=%d", beforeOffset, m.staged.viewport.YOffset())
	}
}

func TestWToggleSoftWrap(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "this is a very long line that should wrap in narrow diff panes\n")

	m := New(repo)
	m.ready = true
	m.width = 80
	m.height = 20
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.syncDiffViewports()

	wrappedCount := len(m.unstaged.viewLines)
	if !m.wrapSoft {
		t.Fatal("expected wrapSoft enabled by default")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = updated.(Model)
	if m.wrapSoft {
		t.Fatal("expected wrapSoft disabled after w")
	}
	unwrappedCount := len(m.unstaged.viewLines)
	if unwrappedCount > wrappedCount {
		t.Fatalf("expected unwrapped lines <= wrapped lines, got wrapped=%d unwrapped=%d", wrappedCount, unwrappedCount)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = updated.(Model)
	if !m.wrapSoft {
		t.Fatal("expected wrapSoft enabled after second w")
	}
}

func TestBinaryFileShowsSizeSummaryInsteadOfNoFileSelected(t *testing.T) {
	repo := testutil.TempRepo(t)
	path := repo + "/bin.dat"
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03}, 0644); err != nil {
		t.Fatalf("write baseline binary: %v", err)
	}
	testutil.MustGitExported(t, repo, "add", "bin.dat")
	testutil.MustGitExported(t, repo, "commit", "-m", "add binary")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}, 0644); err != nil {
		t.Fatalf("write modified binary: %v", err)
	}

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.syncDiffViewports()

	view := ansi.Strip(m.renderDiffPane(80, 12))
	if strings.Contains(view, "No file selected") {
		t.Fatalf("expected binary summary instead of no-file message, got:\n%s", view)
	}
	if !strings.Contains(view, "binary file (prev size: 4 B, new size: 7 B)") {
		t.Fatalf("expected binary size summary, got:\n%s", view)
	}
}

func TestDefaultSettingsEnableNerdFontIcons(t *testing.T) {
	settings := DefaultSettings()
	if !settings.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true")
	}
}

func TestStatusEntryColor(t *testing.T) {
	tests := []struct {
		name  string
		entry statusEntry
		want  string
	}{
		{name: "unstaged", entry: statusEntry{HasUnstaged: true}, want: "#cdd6f4"},
		{name: "partial", entry: statusEntry{HasStaged: true, HasUnstaged: true}, want: "#fab387"},
		{name: "staged", entry: statusEntry{HasStaged: true}, want: "#a6e3a1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusEntryColor(tt.entry); got != tt.want {
				t.Fatalf("statusEntryColor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusEntryMeta_NerdFont(t *testing.T) {
	icons := statusPaneIconsFor(true)

	if got := statusEntryMeta(statusEntry{HasStaged: true, HasUnstaged: true}, true, icons); got != "" {
		t.Fatalf("partial nerd icon = %q", got)
	}
	if got := statusEntryMeta(statusEntry{HasStaged: true}, true, icons); got != "" {
		t.Fatalf("staged nerd icon = %q", got)
	}
}

func TestStatusFileIcon(t *testing.T) {
	icons := statusPaneIconsFor(true)

	if got := statusFileIcon(git.StageFileStatus{IndexStatus: '?', WorktreeCode: '?'}, false, icons); got != "" {
		t.Fatalf("untracked icon = %q, want new file icon", got)
	}
	if got := statusFileIcon(git.StageFileStatus{IndexStatus: 'A', WorktreeCode: ' '}, false, icons); got != "" {
		t.Fatalf("added icon = %q, want new file icon", got)
	}
	if got := statusFileIcon(git.StageFileStatus{IndexStatus: ' ', WorktreeCode: 'M'}, false, icons); got != "" {
		t.Fatalf("modified icon = %q, want modified file icon", got)
	}
}

func TestStatusFileIconSymlink(t *testing.T) {
	nerd := statusPaneIconsFor(true)
	plain := statusPaneIconsFor(false)

	modified := git.StageFileStatus{Path: "link", IndexStatus: ' ', WorktreeCode: 'M'}
	if got := statusFileIcon(modified, true, nerd); got != "󰌷" {
		t.Fatalf("symlink nerd icon = %q, want symlink icon", got)
	}
	if got := statusFileIcon(modified, true, plain); got != "L" {
		t.Fatalf("symlink plain icon = %q, want L", got)
	}

	// Deleted symlink keeps the delete icon (deletion takes precedence).
	deleted := git.StageFileStatus{Path: "link", IndexStatus: 'D'}
	if got := statusFileIcon(deleted, true, nerd); got == "󰌷" {
		t.Fatalf("deleted symlink should show delete icon, not symlink icon")
	}
}

func TestLineModeCanUnstageSingleModifiedLine(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "old-1\nold-2\nold-3\n")
	testutil.MustGitExported(t, repo, "add", "README.md")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "README.md", "new-1\nnew-2\nnew-3\n")

	m := New(repo)
	m.ready = true

	// Stage everything first.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	// Enter diff view and switch to line mode.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	// Move to second changed line and unstage it.
	for range 4 {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := git.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	unstaged, err := git.DiffPath(repo, "README.md", false, 1)
	if err != nil {
		t.Fatalf("unstaged diff: %v", err)
	}

	if !strings.Contains(staged, "+new-1") || strings.Contains(staged, "+new-2") || !strings.Contains(unstaged, "+new-2") {
		t.Fatalf("unexpected diffs after unstage line; status=%q\nSTAGED:\n%s\nUNSTAGED:\n%s", m.statusMsg, staged, unstaged)
	}
}

func TestLineModeStagesSingleLineInUntrackedFile(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "new.txt", "line-1\nline-2\nline-3\n")

	m := New(repo)
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := git.DiffPath(repo, "new.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if strings.Contains(staged, "+line-1") || !strings.Contains(staged, "+line-2") || strings.Contains(staged, "+line-3") {
		t.Fatalf("expected single line staged for untracked file; status=%q\nSTAGED:\n%s", m.statusMsg, staged)
	}
}

func TestLineModeUnstageOneOfAdjacentDeletedLinesDoesNotDuplicate(t *testing.T) {
	repo := testutil.TempRepo(t)
	base := strings.Join([]string{
		"func f() {",
		"    if cond {",
		"        x()",
		"    }",
		"    y()",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "f.go", base)
	testutil.MustGitExported(t, repo, "add", "f.go")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := strings.Join([]string{
		"func f() {",
		"    if cond {",
		"    y()",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "f.go", updated)

	m := New(repo)
	m.ready = true

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)

	staged, err := git.DiffPath(repo, "f.go", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if strings.Contains(staged, "+    }") {
		t.Fatalf("unexpected duplicated closing brace in staged diff:\n%s", staged)
	}
}

func TestLineModeUnstageBraceFromFirstHunkBlock(t *testing.T) {
	repo := testutil.TempRepo(t)
	base := strings.Join([]string{
		"package git",
		"",
		"func DiffUntrackedPath(worktreeRoot, path string, color bool, contextLines int) (string, error) {",
		"    diffPath := path",
		"    absPath := path",
		"    if !filepath.IsAbs(path) {",
		"        absPath = filepath.Join(worktreeRoot, path)",
		"    }",
		"",
		"    if !color {",
		"        return \"\", nil",
		"    }",
		"    _ = absPath",
		"    return \"\", nil",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "stage.go", base)
	testutil.MustGitExported(t, repo, "add", "stage.go")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := strings.Join([]string{
		"package git",
		"",
		"func DiffUntrackedPath(worktreeRoot, path string, color bool, contextLines int) (string, error) {",
		"    diffPath := path",
		"",
		"    if !color {",
		"        return \"\", nil",
		"    }",
		"    return \"\", nil",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "stage.go", updated)

	m := New(repo)
	m.ready = true

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updatedModel.(Model)

	// Move to the deletion of the closing brace line.
	for i := 0; i < 3; i++ {
		updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updatedModel.(Model)
	}
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)

	staged, err := git.DiffPath(repo, "stage.go", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if strings.Contains(staged, "+    }") {
		t.Fatalf("unexpected duplicated brace in staged diff:\n%s", staged)
	}
}

func TestVisualModeStagesLineRange(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "range.txt", "one\ntwo\nthree\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	if !m.unstaged.visualActive {
		t.Fatalf("expected visual mode active after v")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := git.DiffPath(repo, "range.txt", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if !strings.Contains(staged, "+one") || !strings.Contains(staged, "+two") {
		t.Fatalf("expected staged diff to include selected visual range:\n%s", staged)
	}
	if m.unstaged.visualActive {
		t.Fatalf("expected visual mode to exit after applying selection")
	}
}

func TestEscExitsVisualModeAndKeepsDiffFocus(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "range.txt", "one\ntwo\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	if !m.unstaged.visualActive {
		t.Fatalf("expected visual mode active after v")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)

	if m.focus != focusDiff {
		t.Fatalf("expected esc in visual mode to keep diff focus")
	}
	if m.unstaged.visualActive {
		t.Fatalf("expected esc to exit visual mode")
	}
}

func TestDiffDotMovesToNextFile(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged

	before, ok := m.selectedFile()
	if !ok {
		t.Fatalf("expected selected file before navigation")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)

	after, ok := m.selectedFile()
	if !ok {
		t.Fatalf("expected selected file after navigation")
	}
	if after.Path == before.Path {
		t.Fatalf("expected '.' to move to next file, stayed on %q", after.Path)
	}
	if m.focus != focusDiff {
		t.Fatalf("expected to remain in diff focus, got %v", m.focus)
	}
}

func TestDiffCommaMovesToPreviousFile(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged

	first, ok := m.selectedFile()
	if !ok {
		t.Fatalf("expected selected file")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	m = updated.(Model)

	back, ok := m.selectedFile()
	if !ok {
		t.Fatalf("expected selected file after ','")
	}
	if back.Path != first.Path {
		t.Fatalf("expected ',' to return to previous file %q, got %q", first.Path, back.Path)
	}
}

func TestVisualModeUnstagesLineRange(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "range.txt", "one\ntwo\nthree\n")

	m := New(repo)
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	m.focus = focusDiff
	m.section = sectionStaged
	m.navMode = navLine

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := git.DiffPath(repo, "range.txt", true, 1)
	if err != nil {
		t.Fatalf("DiffPath(staged): %v", err)
	}
	unstaged, err := git.DiffPath(repo, "range.txt", false, 1)
	if err != nil {
		t.Fatalf("DiffPath(unstaged): %v", err)
	}

	if strings.Contains(staged, "+one") || strings.Contains(staged, "+two") || !strings.Contains(staged, "+three") {
		t.Fatalf("expected staged diff to keep only third line:\n%s", staged)
	}
	if !strings.Contains(unstaged, "+one") || !strings.Contains(unstaged, "+two") {
		t.Fatalf("expected unstaged diff to include selected visual range:\n%s", unstaged)
	}
}
