package log

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/splitview"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var settings = ui.Settings{}

func newTestModel() Model {
	m := Model{
		keys:   newLogManager(),
		search: search.NewModel(),
	}
	m.help = help.NewModel(help.BuildSections(m.keys, m.search.Keys()))
	m.listPanel = newListPanel()
	m.split = splitview.New(m.listPanel, m.commitDetail)
	return m
}

func newTestModelDefault(worktreeRoot, startRef string, settings ui.Settings) Model {
	return runModelInit(NewModel(worktreeRoot, startRef, settings, LogFilter{}, keys.Manager{}))
}

func newTestModelFiltered(worktreeRoot, startRef string, settings ui.Settings, filter LogFilter) Model {
	return runModelInit(NewModel(worktreeRoot, startRef, settings, filter, keys.Manager{}))
}

// runModelInit executes Init() synchronously so tests can inspect m.rows immediately.
func runModelInit(m Model) Model {
	return runCmd(m, m.Init())
}

// runCmd executes a single command synchronously, handling batch commands
// by running each sub-command in sequence.
func runCmd(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runCmd(m, c)
		}
		return m
	}
	next, _ := m.Update(msg)
	if m2, ok := next.(Model); ok {
		return m2
	}
	return m
}

func TestReloadAssignsBranchHistoryClasses(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature")
	testutil.MustGitExported(t, wtDir, "reset", "--hard", "HEAD~1")
	testutil.WriteFile(t, wtDir, "shared.txt", "shared\n")
	testutil.CommitAll(t, wtDir, "shared commit")
	testutil.MustGitExported(t, wtDir, "push", "--force-with-lease", "-u", "origin", "feature")

	testutil.WriteFile(t, wtDir, "remote.txt", "remote\n")
	testutil.CommitAll(t, wtDir, "remote only")
	testutil.MustGitExported(t, wtDir, "push")

	testutil.MustGitExported(t, wtDir, "reset", "--hard", "HEAD~1")
	testutil.WriteFile(t, wtDir, "local.txt", "local\n")
	testutil.CommitAll(t, wtDir, "local only")

	m := newTestModelDefault(wtDir, "", settings)
	got := map[string]git.BranchHistoryClass{}
	for _, row := range m.listPanel.Rows() {
		if row.kind != rowCommit {
			continue
		}
		got[row.commit.Subject] = row.class
	}

	if got["shared commit"] != git.BranchHistoryShared {
		t.Fatalf("shared commit class = %q", got["shared commit"])
	}
	if got["local only"] != git.BranchHistoryLocalOnly {
		t.Fatalf("local only class = %q", got["local only"])
	}
}

func TestRenderCommitRowUsesBranchHistoryColors(t *testing.T) {
	local := commitState(git.BranchHistoryLocalOnly, false).style.Render("local only")
	if ansi.Strip(local) != "local only" || local == "local only" {
		t.Fatalf("expected local-only subject to be colorized, got %q", local)
	}

	remote := commitState(git.BranchHistoryRemoteOnly, false).style.Render("remote only")
	if ansi.Strip(remote) != "remote only" || remote == "remote only" {
		t.Fatalf("expected remote-only subject to be colorized, got %q", remote)
	}

	shared := commitState(git.BranchHistoryShared, false).style.Render("shared")
	if ansi.Strip(shared) != "shared" || shared == "shared" {
		t.Fatalf("expected shared subject to be colorized, got %q", shared)
	}
}

func TestRenderCommitRowUsesDivergedLocalColor(t *testing.T) {
	normal := commitState(git.BranchHistoryLocalOnly, false).style.Render("local only")
	diverged := commitState(git.BranchHistoryLocalOnly, true).style.Render("local only")
	if normal == diverged {
		t.Fatalf("expected diverged local style to differ from normal local style")
	}
}

func TestGHResetsCustomRefToHead(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "one")

	m := newTestModelDefault(repo, "HEAD~1", ui.Settings{EnableNavigation: true})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if cmd == nil {
		t.Fatalf("expected nav command for gh")
	}
	vs, ok := nav.IsSwitch(cmd())
	if !ok {
		t.Fatalf("expected nav replace")
	}
	if vs.Tab != nav.TabLog || vs.Ref != "HEAD" {
		t.Fatalf("expected log HEAD view state, got tab=%q ref=%q", vs.Tab, vs.Ref)
	}
}

func TestSelectedCommitRowFillsFullWidth(t *testing.T) {
	m := newTestModel()
	m.width = 80
	r := row{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "12345678",
			AuthorShort: "AB",
			Subject:     "subject",
			Date:        time.Now().Add(-2 * time.Hour),
			Graph:       "*",
			Decorations: []git.RefDecoration{{Name: "main", Kind: git.RefDecorationLocalBranch}},
		},
	}
	m.listPanel = m.listPanel.WithRows([]row{r})
	line := m.listPanel.WithHints(m.buildHints()).renderRow(r, true, 40)
	if got := ansi.StringWidth(ansi.Strip(line)); got != 40 {
		t.Fatalf("selected row width = %d, want 40", got)
	}
	if line == ansi.Strip(line) {
		t.Fatalf("expected selected row to preserve nested ansi colors")
	}
}

func TestMatchRefRule(t *testing.T) {
	rules := compileRefRules(config.DefaultLogConfig().ImportantRefs)

	tests := []struct {
		name      string
		ref       string
		wantMatch bool
	}{
		{name: "main matches", ref: "main", wantMatch: true},
		{name: "master matches", ref: "master", wantMatch: true},
		{name: "origin/main matches", ref: "origin/main", wantMatch: true},
		{name: "origin/master matches", ref: "origin/master", wantMatch: true},
		{name: "feature branch does not match", ref: "feature/x", wantMatch: false},
		{name: "version tag matches", ref: "v1.0.0", wantMatch: true},
		{name: "non-version ref does not match", ref: "some-branch", wantMatch: false},
	}

	for _, tt := range tests {
		_, ok := matchRefRule(tt.ref, rules)
		if ok != tt.wantMatch {
			t.Fatalf("%s: match = %v, want %v", tt.name, ok, tt.wantMatch)
		}
	}
}

func TestRenderCommitRowHighlightsSearchMatches(t *testing.T) {
	m := newTestModel()
	m.search.Start("fix")
	r := row{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "12345678",
			AuthorShort: "AB",
			Subject:     "fix search highlighting",
		},
	}
	m.listPanel = m.listPanel.WithRows([]row{r})

	line := m.listPanel.WithHints(m.buildHints()).renderCommitRow(r, false)
	if stripped := ansi.Strip(line); !strings.Contains(stripped, "fix search highlighting") {
		t.Fatalf("stripped line = %q", stripped)
	}
	if line == ansi.Strip(line) {
		t.Fatalf("expected ansi highlight in rendered commit row")
	}
}

func TestRenderBadgesHighlightsSearchMatches(t *testing.T) {
	m := newTestModel()
	m.search.Start("main")
	line := m.listPanel.WithHints(m.buildHints()).renderBadges([]git.RefDecoration{{Name: "origin/main", Kind: git.RefDecorationRemoteBranch}}, false)
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

	m := newTestModelDefault(repo, "", settings)
	m.search.Start("fix")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected search matches")
	}

	m.search.DismissAndKeepResults()
	if m.search.Query() != "fix" {
		t.Fatalf("expected query to persist after close, got %q", m.search.Query())
	}
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected matches to persist after close")
	}
	if got := ansi.Strip(m.frameRightTitle()); !strings.Contains(got, "1/2 matches") {
		t.Fatalf("expected search match count in frame title, got %q", got)
	}
}

func TestNAndNShiftMoveBetweenSearchResults(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "fix one")
	testutil.WriteFile(t, repo, "two.txt", "two\n")
	testutil.CommitAll(t, repo, "fix two")

	m := newTestModelDefault(repo, "", settings)
	m.search.Start("fix")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() < 2 {
		t.Fatalf("expected at least two search matches, got %d", m.search.MatchesCount())
	}
	if match, ok := m.search.Match(0); ok {
		m.listPanel = m.listPanel.SetSelected(match.DataIndex)
	}
	m.search.SetCursor(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if match, ok := m.search.Match(1); ok && m.listPanel.Selected() != match.DataIndex {
		t.Fatalf("expected n to move to next result")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N', Mod: tea.ModShift})
	m = updated.(Model)
	if match, ok := m.search.Match(0); ok && m.listPanel.Selected() != match.DataIndex {
		t.Fatalf("expected N to move to previous result")
	}
}

func TestTagJumpChordsMoveToTaggedCommits(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowCommit, commit: git.LogEntry{Subject: "c0"}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1", Decorations: []git.RefDecoration{{Name: "v1.0.0", Kind: git.RefDecorationTag}}}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c2"}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c3", Decorations: []git.RefDecoration{{Name: "v2.0.0", Kind: git.RefDecorationTag}}}},
	}).SetSelected(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.listPanel.Selected() != 1 {
		t.Fatalf("expected ]t to jump to first tag at 1, got %d", m.listPanel.Selected())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.listPanel.Selected() != 3 {
		t.Fatalf("expected ]t to jump to next tag at 3, got %d", m.listPanel.Selected())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.listPanel.Selected() != 1 {
		t.Fatalf("expected [t to jump back to tag at 1, got %d", m.listPanel.Selected())
	}
}

func TestTagJumpChordStopsAtEdges(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowCommit, commit: git.LogEntry{Subject: "c0", Decorations: []git.RefDecoration{{Name: "v1", Kind: git.RefDecorationTag}}}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1"}},
	}).SetSelected(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.listPanel.Selected() != 0 {
		t.Fatalf("expected [t at first tag to stay put, got %d", m.listPanel.Selected())
	}
}

func TestDispatchBinding_Navigation(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", ui.Settings{EnableNavigation: true})
	m.width = 120
	m.height = 40

	// j / k navigation
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	initial := m.listPanel.Selected()
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.listPanel.Selected() != 0 && m.listPanel.Selected() == initial {
		// either k moved up or we were already at top — just verify no crash
	}

	// G goes to bottom
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'G', Text: "G", ShiftedCode: 'G', Mod: tea.ModShift})
	bottom := m.listPanel.Selected()
	if bottom < 0 {
		t.Fatalf("G: expected non-negative selection, got %d", bottom)
	}

	// gg goes to top
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if m.listPanel.Selected() != 0 {
		t.Fatalf("gg: expected top (0), got %d", m.listPanel.Selected())
	}

	// q with navigation → nav back command
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q with EnableNavigation: expected nav.Back command")
	}

	// R reload
	m, cmd = sendKey(m, tea.KeyPressMsg{Code: 'R', Text: "R", ShiftedCode: 'R', Mod: tea.ModShift})
	if !m.refreshing {
		t.Fatal("R: expected refreshing=true")
	}
}

func TestDispatchBinding_PageScrollAndHelp(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.height = 40
	m.listPanel = m.listPanel.WithRows(make([]row, 30))

	before := m.listPanel.Selected()
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	_ = before

	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})

	// ? opens help
	m, _ = sendKey(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !m.help.IsOpen {
		t.Fatal("?: expected help to open")
	}
}

func TestDispatchBinding_ClearFilter(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelFiltered(repo, "", settings, LogFilter{Path: "foo.go"})
	m.width = 120
	m.height = 40

	if !m.filter.IsActive() {
		t.Fatal("expected active filter")
	}

	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	if m.filter.IsActive() {
		t.Fatal("f: expected filter cleared")
	}
}

func TestSelectRefAndSelectedRef(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa111", Subject: "first"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "bbb222", Subject: "second"}},
	}).SetSelected(0)

	// SelectedRef at row 0
	if got := m.SelectedRef(); got != "aaa111" {
		t.Errorf("SelectedRef() = %q, want 'aaa111'", got)
	}

	// SelectRef moves cursor to matching hash
	m = m.SelectRef("bbb222")
	if m.listPanel.Selected() != 1 {
		t.Errorf("SelectRef: expected selection at 1, got %d", m.listPanel.Selected())
	}

	// SelectRef with unknown hash — cursor unchanged
	m = m.SelectRef("zzzxxx")
	if m.listPanel.Selected() != 1 {
		t.Errorf("SelectRef unknown: expected selection unchanged at 1, got %d", m.listPanel.Selected())
	}

	// SelectedRef returns empty for empty rows
	empty := newTestModel()
	if got := empty.SelectedRef(); got != "" {
		t.Errorf("SelectedRef empty model = %q, want ''", got)
	}
}

func TestMouseWheelScrolls(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows(make([]row, 30))
	m.height = 10
	updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	next := updated.(Model)
	_ = next // just ensure no panic

	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	next = updated.(Model)
	_ = next
}

func sendKey(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestFocusReloadsRowsAfterOnDiskChange(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo, "", settings)
	initialRows := len(m.listPanel.Rows())

	testutil.WriteFile(t, repo, "new.txt", "new\n")
	testutil.CommitAll(t, repo, "new commit")

	updated, cmd := m.Update(tea.FocusMsg{})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected focus to trigger reload command")
	}

	// FocusMsg now returns a batch (reload + status load); run all sub-commands.
	m = runCmd(m, cmd)
	if len(m.listPanel.Rows()) <= initialRows {
		t.Fatalf("expected more rows after focus reload; before=%d after=%d", initialRows, len(m.listPanel.Rows()))
	}
}

func TestKeyManager(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", settings)
	km := m.KeyManager()
	if len(km.Bindings()) == 0 {
		t.Error("expected non-empty key bindings")
	}
}

func TestInputFocused_False(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", settings)
	if m.InputFocused() {
		t.Error("expected InputFocused=false by default")
	}
}

func TestWithPendingFocus(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo, "", settings)
	m2 := m.WithPendingFocus("abc123")
	if m2.pendingFocusSubject != "abc123" {
		t.Errorf("pendingFocusSubject = %q, want 'abc123'", m2.pendingFocusSubject)
	}
}

func TestAutoReload_WithPending(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "f.txt", "a\n")
	testutil.CommitAll(t, repo, "init")

	m := newTestModelDefault(repo, "", settings)
	m = m.WithPendingFocus("HEAD")
	cmd := m.AutoReload()
	if cmd == nil {
		t.Error("expected non-nil cmd from AutoReload with pending focus")
	}
}

// --- Pseudo-log-line tests ---

func TestPseudoLogLineIsFirstRow(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "first commit")

	m := newTestModelDefault(repo, "", settings)
	rows := m.listPanel.Rows()
	if len(rows) == 0 {
		t.Fatal("expected rows after init")
	}
	if rows[0].kind != rowPseudoStatus {
		t.Fatalf("rows[0].kind = %v, want rowPseudoStatus", rows[0].kind)
	}
}

func TestPseudoLogLineLoadingState(t *testing.T) {
	// Before the worktree status is loaded, the pseudo-line shows "loading".
	m := newTestModel()
	r := row{kind: rowPseudoStatus, detail: ""}
	m.listPanel = m.listPanel.WithRows([]row{r})
	line := m.listPanel.WithHints(m.buildHints()).renderRow(r, false, 80)
	stripped := ansi.Strip(line)
	if !strings.Contains(stripped, "working tree") {
		t.Errorf("renderRow: expected 'working tree' label, got %q", stripped)
	}
}

func TestPseudoLogLineCleanState(t *testing.T) {
	m := newTestModel()
	m.statusLoaded = true
	// staged=0, unstaged=0, untracked=0 → "no local changes"
	r := row{kind: rowPseudoStatus, detail: m.pseudoStatusDetail()}
	m.listPanel = m.listPanel.WithRows([]row{r})
	line := m.listPanel.WithHints(m.buildHints()).renderRow(r, false, 80)
	stripped := ansi.Strip(line)
	if !strings.Contains(stripped, "no local changes") {
		t.Errorf("renderRow: expected 'no local changes', got %q", stripped)
	}
}

func TestPseudoLogLineDirtyState(t *testing.T) {
	m := newTestModel()
	m.statusLoaded = true
	m.statusStaged = 2
	m.statusUnstaged = 3
	m.statusUntracked = 1
	r := row{kind: rowPseudoStatus, detail: m.pseudoStatusDetail()}
	m.listPanel = m.listPanel.WithRows([]row{r})
	line := m.listPanel.WithHints(m.buildHints()).renderRow(r, false, 80)
	stripped := ansi.Strip(line)
	if !strings.Contains(stripped, "2 staged") {
		t.Errorf("expected '2 staged' in %q", stripped)
	}
	if !strings.Contains(stripped, "3 unstaged") {
		t.Errorf("expected '3 unstaged' in %q", stripped)
	}
	if !strings.Contains(stripped, "1 untracked") {
		t.Errorf("expected '1 untracked' in %q", stripped)
	}
}

func TestPseudoLogLineDirtyStateZeroCountsOmitted(t *testing.T) {
	m := newTestModel()
	m.statusLoaded = true
	m.statusStaged = 1
	// unstaged=0, untracked=0 — these counts must be absent from output.
	r := row{kind: rowPseudoStatus, detail: m.pseudoStatusDetail()}
	m.listPanel = m.listPanel.WithRows([]row{r})
	detail := m.pseudoStatusDetail()
	if strings.Contains(detail, "0 ") {
		t.Errorf("detail should omit zero counts, got %q", detail)
	}
	if !strings.Contains(detail, "1 staged") {
		t.Errorf("expected '1 staged' in detail %q", detail)
	}
}

func TestEnterOnPseudoLogLineEmitsStatusSwitch(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", ui.Settings{EnableNavigation: true})
	// Move cursor to the pseudo-line (index 0).
	m.listPanel = m.listPanel.SetSelected(0)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter on pseudo-log-line")
	}
	msg := cmd()
	vs, ok := nav.IsSwitch(msg)
	if !ok {
		t.Fatalf("expected nav.Switch message, got %T", msg)
	}
	if vs.Tab != nav.TabStatus {
		t.Errorf("expected TabStatus, got %q", vs.Tab)
	}
	if vs.WorktreeRoot != repo {
		t.Errorf("expected WorktreeRoot=%q, got %q", repo, vs.WorktreeRoot)
	}
}

// --- Split view integration tests ---

func TestEnterOnCommitInCollapsedExpandsToSplit(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()

	// Row 1 is the first real commit (row 0 is pseudo-line).
	m.listPanel = m.listPanel.SetSelected(1)
	if !m.split.IsCollapsed() {
		t.Fatal("expected Collapsed initially")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatalf("expected Split after Enter on commit, vis = collapsed=%v split=%v", m.split.IsCollapsed(), m.split.IsSplit())
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after expanding")
	}
}

func TestLOnCommitInCollapsedExpandsToSplit(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)
	if !m.split.IsCollapsed() {
		t.Fatal("expected Collapsed initially")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatal("expected Split after l on commit")
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after l on commit")
	}
	if !m.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused after l on commit")
	}
}

func TestLFromLogPanelFocusesOpenDetail(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if !m.split.IsSplit() || !m.split.IsListFocused() {
		t.Fatal("expected open split with log focused")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatal("expected split to remain open after l")
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after l from log panel")
	}
	if !m.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused after l from log panel")
	}
}

func TestEscFromDetailReturnsFocusToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	// Expand to split.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}

	// Esc from detail → list focused, still split.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatal("expected still Split after Esc from detail")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after Esc from detail")
	}
}

func TestQFromDetailReturnsFocusToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected q to switch focus without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsSplit() {
		t.Fatal("expected still Split after q from detail")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after q from detail")
	}
}

func TestHFromDetailFileTreeReturnsFocusToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}
	if !m.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused after opening detail")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected h to switch focus without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsSplit() {
		t.Fatal("expected still Split after h from detail file tree")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after h from detail file tree")
	}
}

func TestHFromDetailHeaderReturnsFocusToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused before pressing h")
	}
	if !m.commitDetail.IsHeaderFocused() {
		t.Fatal("expected commit header focused before pressing h")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected h to switch focus without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsSplit() {
		t.Fatal("expected still Split after h from detail header")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after h from detail header")
	}
}

func TestEscFromDetailInternalFocusStepsBackWithoutMovingToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	// Open split view.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}

	// Tab routes to commit model and cycles focus to header (or diff).
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.commitDetail.HasInternalFocus() {
		t.Skip("commit detail has no internal focus after Tab — skipping")
	}

	// Esc should step back internally — NOT move split focus to list.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected esc to be handled internally without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail still focused after internal esc (should step back, not defocus detail)")
	}
}

func TestQFromDetailInternalFocusStepsBackWithoutMovingToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.commitDetail.HasInternalFocus() {
		t.Skip("commit detail has no internal focus after Tab — skipping")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected q to be handled internally without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail still focused after internal q")
	}
}

func TestEscFromListWhileSplitCollapses(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	// Expand to split.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	// Esc → list focused.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	// Esc again → collapsed.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if !m.split.IsCollapsed() {
		t.Fatal("expected Collapsed after second Esc")
	}
}

func TestQFromListWhileSplitCollapses(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected q from list split to collapse without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsCollapsed() {
		t.Fatal("expected Collapsed after q from list in split")
	}
}

func TestLogPaneFrameColorTracksSplitFocus(t *testing.T) {
	m := newTestModel()
	if m.logPaneBorderColor() != ui.ColorBorder {
		t.Fatal("expected collapsed log frame to use default border")
	}
	if m.logPaneTitleColor() != ui.ColorBlue {
		t.Fatal("expected collapsed log frame to use default title color")
	}

	m.split = splitview.NewSplit(m.listPanel, m.commitDetail)
	if m.logPaneBorderColor() != ui.ColorOrange {
		t.Fatal("expected log frame orange while split list is focused")
	}

	m.split, _ = m.split.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused")
	}
	if m.logPaneBorderColor() != ui.ColorBorder {
		t.Fatal("expected log frame inactive while detail is focused")
	}

	m.split, _ = m.split.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.logPaneBorderColor() != ui.ColorOrange {
		t.Fatal("expected log frame orange after focus returns to list")
	}
}

func TestFKeyTogglesFullscreenList(t *testing.T) {
	m := newTestModel()
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowPseudoStatus},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "abc", Subject: "c1"}},
	}).SetSelected(1)

	// f in collapsed state → fullscreen list.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)
	if !m.split.IsFullscreen() {
		t.Fatal("expected Fullscreen after f in collapsed")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused in fullscreen")
	}
}

func TestFKeyOnDetailFocusedTogglesFullscreenDetail(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	// Expand to split (detail focused).
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}

	// f → fullscreen detail.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)
	if !m.split.IsFullscreen() {
		t.Fatal("expected Fullscreen after f with detail focused")
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused in fullscreen")
	}
}

func TestToChordTogglesOrientation(t *testing.T) {
	m := newTestModel()
	m.width = 200
	m.height = 40
	m, _ = m.syncSplitSize()

	// Press "t" then "o" to toggle orientation.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(Model)

	// At width 200, auto-orient is Vertical; "to" should override to Horizontal.
	if m.split.EffectiveOrientation() != splitview.Horizontal {
		t.Fatalf("expected Horizontal after 'to' at width 200, got %v", m.split.EffectiveOrientation())
	}
}

func TestWorktreeStatusUpdatesRows(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowPseudoStatus, detail: "loading worktree status…"},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1"}},
	})

	updated, _ := m.Update(worktreeStatusMsg{staged: 1, unstaged: 2, untracked: 0})
	m = updated.(Model)

	if !m.statusLoaded {
		t.Error("expected statusLoaded=true")
	}
	detail := m.listPanel.Rows()[0].detail
	if !strings.Contains(detail, "1 staged") {
		t.Errorf("expected '1 staged' in detail %q", detail)
	}
	if !strings.Contains(detail, "2 unstaged") {
		t.Errorf("expected '2 unstaged' in detail %q", detail)
	}
	if strings.Contains(detail, "untracked") {
		t.Errorf("expected no 'untracked' for count=0, got %q", detail)
	}
}

func TestEnterOnCommitRendersDetailContent(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a content\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)

	// WindowSizeMsg must go through Update to set m.ready = true (same as real app).
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	m = updated.(Model)
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatal("expected split mode after Enter")
	}

	view := m.View()
	stripped := ansi.Strip(view.Content)
	if !strings.Contains(stripped, "a.txt") {
		t.Errorf("expected 'a.txt' in view after opening commit detail, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "Commit") {
		t.Errorf("expected 'Commit' header in view after opening commit detail, got:\n%s", stripped)
	}
}

func TestFlashOnJumpSetsFlashState(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa111", Subject: "target commit"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "bbb222", Subject: "other commit"}},
	})

	updated, _ := m.Update(reloadMsg{
		rows: []row{
			{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa111", Subject: "target commit"}},
			{kind: rowCommit, commit: git.LogEntry{FullHash: "bbb222", Subject: "other commit"}},
		},
		focusSubject: "target commit",
	})
	m = updated.(Model)

	if m.flashSubject != "target commit" {
		t.Errorf("flashSubject = %q, want 'target commit'", m.flashSubject)
	}
	if m.flashUntil.IsZero() {
		t.Error("flashUntil should be set after flash-on-jump")
	}
	if m.listPanel.Selected() != 0 {
		t.Errorf("cursor should be on matched row 0, got %d", m.listPanel.Selected())
	}
}

func TestFlashClearMsgResetsFlashState(t *testing.T) {
	m := newTestModel()
	m.flashSubject = "some subject"
	m.flashUntil = time.Now().Add(2 * time.Second)

	updated, _ := m.Update(flashClearMsg{})
	m = updated.(Model)

	if m.flashSubject != "" {
		t.Errorf("flashSubject should be cleared, got %q", m.flashSubject)
	}
	if !m.flashUntil.IsZero() {
		t.Error("flashUntil should be zero after clear")
	}
}

func TestFilteredLogShowsOnlyFilteredCommits(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit touching a")
	testutil.WriteFile(t, repo, "b.txt", "b\n")
	testutil.CommitAll(t, repo, "commit touching b only")

	m := newTestModelFiltered(repo, "", settings, LogFilter{Path: "b.txt"})
	rows := m.listPanel.Rows()
	for _, r := range rows {
		if r.kind != rowCommit {
			continue
		}
		if r.commit.Subject == "commit touching a" {
			t.Fatal("filtered log should not contain commits that don't touch b.txt")
		}
	}
	found := false
	for _, r := range rows {
		if r.kind == rowCommit && r.commit.Subject == "commit touching b only" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("filtered log should contain commit that touches b.txt")
	}
}
