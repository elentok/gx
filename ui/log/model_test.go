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
	return m
}

func newTestModelDefault(worktreeRoot, startRef string, settings ui.Settings) Model {
	return NewModel(worktreeRoot, startRef, settings, LogFilter{}, keys.Manager{})
}

func newTestModelFiltered(worktreeRoot, startRef string, settings ui.Settings, filter LogFilter) Model {
	return NewModel(worktreeRoot, startRef, settings, filter, keys.Manager{})
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
	for _, row := range m.rows {
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

func TestEnterOnCommitRowOpensCommitView(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo, "", settings)
	for i := range m.rows {
		if m.rows[i].kind == rowCommit {
			m.list.SetSelected(i, len(m.rows))
			break
		}
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected nav command on enter")
	}
	vs, ok := nav.IsOpen(cmd())
	if !ok {
		t.Fatalf("expected nav push")
	}
	if vs.Tab != nav.TabCommit {
		t.Fatalf("expected commit tab, got %q", vs.Tab)
	}
	_ = updated
}

func TestEnterOnCommitRowCarriesActiveFilterPath(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, filepath.Join(repo, "src"))
	testutil.WriteFile(t, repo, "src/main.go", "package main\n")
	testutil.CommitAll(t, repo, "add file")

	m := newTestModelFiltered(repo, "", settings, LogFilter{Path: "src/main.go"})
	for i := range m.rows {
		if m.rows[i].kind == rowCommit {
			m.list.SetSelected(i, len(m.rows))
			break
		}
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected nav command on enter")
	}
	vs, ok := nav.IsOpen(cmd())
	if !ok {
		t.Fatalf("expected nav push")
	}
	if vs.FilterPath != "src/main.go" {
		t.Fatalf("vs.FilterPath = %q, want %q", vs.FilterPath, "src/main.go")
	}
}

func TestSelectedCommitRowFillsFullWidth(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.rows = []row{{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "12345678",
			AuthorShort: "AB",
			Subject:     "subject",
			Date:        time.Now().Add(-2 * time.Hour),
			Graph:       "*",
			Decorations: []git.RefDecoration{{Name: "main", Kind: git.RefDecorationLocalBranch}},
		},
	}}
	line := m.renderRow(m.rows[0], true, 40)
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
	m.rows = []row{{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "12345678",
			AuthorShort: "AB",
			Subject:     "fix search highlighting",
		},
	}}

	line := m.renderCommitRow(m.rows[0])
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
		m.list.SetSelected(match.Index, len(m.rows))
	}
	m.search.SetCursor(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if match, ok := m.search.Match(1); ok && m.list.Selected() != match.Index {
		t.Fatalf("expected n to move to next result")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N', Mod: tea.ModShift})
	m = updated.(Model)
	if match, ok := m.search.Match(0); ok && m.list.Selected() != match.Index {
		t.Fatalf("expected N to move to previous result")
	}
}

func TestTagJumpChordsMoveToTaggedCommits(t *testing.T) {
	m := newTestModel()
	m.rows = []row{
		{kind: rowCommit, commit: git.LogEntry{Subject: "c0"}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1", Decorations: []git.RefDecoration{{Name: "v1.0.0", Kind: git.RefDecorationTag}}}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c2"}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c3", Decorations: []git.RefDecoration{{Name: "v2.0.0", Kind: git.RefDecorationTag}}}},
	}
	m.list.SetSelected(0, len(m.rows))

	updated, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.list.Selected() != 1 {
		t.Fatalf("expected ]t to jump to first tag at 1, got %d", m.list.Selected())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.list.Selected() != 3 {
		t.Fatalf("expected ]t to jump to next tag at 3, got %d", m.list.Selected())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.list.Selected() != 1 {
		t.Fatalf("expected [t to jump back to tag at 1, got %d", m.list.Selected())
	}
}

func TestTagJumpChordStopsAtEdges(t *testing.T) {
	m := newTestModel()
	m.rows = []row{
		{kind: rowCommit, commit: git.LogEntry{Subject: "c0", Decorations: []git.RefDecoration{{Name: "v1", Kind: git.RefDecorationTag}}}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1"}},
	}
	m.list.SetSelected(0, len(m.rows))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.list.Selected() != 0 {
		t.Fatalf("expected [t at first tag to stay put, got %d", m.list.Selected())
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
	initial := m.list.Selected()
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.list.Selected() != 0 && m.list.Selected() == initial {
		// either k moved up or we were already at top — just verify no crash
	}

	// G goes to bottom
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'G', Text: "G", ShiftedCode: 'G', Mod: tea.ModShift})
	bottom := m.list.Selected()
	if bottom < 0 {
		t.Fatalf("G: expected non-negative selection, got %d", bottom)
	}

	// gg goes to top
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if m.list.Selected() != 0 {
		t.Fatalf("gg: expected top (0), got %d", m.list.Selected())
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
	m.rows = make([]row, 30)

	before := m.list.Selected()
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
	m.rows = []row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "aaa111", Subject: "first"}},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "bbb222", Subject: "second"}},
	}
	m.list.SetSelected(0, len(m.rows))

	// SelectedRef at row 0
	if got := m.SelectedRef(); got != "aaa111" {
		t.Errorf("SelectedRef() = %q, want 'aaa111'", got)
	}

	// SelectRef moves cursor to matching hash
	m = m.SelectRef("bbb222")
	if m.list.Selected() != 1 {
		t.Errorf("SelectRef: expected selection at 1, got %d", m.list.Selected())
	}

	// SelectRef with unknown hash — cursor unchanged
	m = m.SelectRef("zzzxxx")
	if m.list.Selected() != 1 {
		t.Errorf("SelectRef unknown: expected selection unchanged at 1, got %d", m.list.Selected())
	}

	// SelectedRef returns empty for empty rows
	empty := newTestModel()
	if got := empty.SelectedRef(); got != "" {
		t.Errorf("SelectedRef empty model = %q, want ''", got)
	}
}

func TestMouseWheelScrolls(t *testing.T) {
	m := newTestModel()
	m.rows = make([]row, 30)
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
	initialRows := len(m.rows)

	testutil.WriteFile(t, repo, "new.txt", "new\n")
	testutil.CommitAll(t, repo, "new commit")

	updated, cmd := m.Update(tea.FocusMsg{})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected focus to trigger reload command")
	}

	reload, ok := cmd().(reloadMsg)
	if !ok {
		t.Fatalf("expected reloadMsg from focus reload cmd")
	}

	updated, _ = m.Update(reload)
	m = updated.(Model)
	if len(m.rows) <= initialRows {
		t.Fatalf("expected more rows after focus reload; before=%d after=%d", initialRows, len(m.rows))
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

func TestOnPageActivated_WithPending(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "f.txt", "a\n")
	testutil.CommitAll(t, repo, "init")

	m := newTestModelDefault(repo, "", settings)
	m = m.WithPendingFocus("HEAD")
	cmd := m.OnPageActivated()
	if cmd == nil {
		t.Error("expected non-nil cmd from OnPageActivated with pending focus")
	}
}
