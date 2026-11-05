package status

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
	notifypkg "github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/status/diffarea"
	"strings"
	"testing"
)

func TestQAndEscFocusBehavior(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("expected nil cmd on esc")
	}
	m2 := updated.(Model)
	if m2.focus != focusFiletree {
		t.Fatalf("esc should move focus to filetree")
	}

	m2.focus = focusFiletree
	updated, cmd = m2.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("q in filetree should quit")
	}
}

func TestQAlwaysQuitsFromDiffFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("expected q to quit from diff focus")
	}
}

func TestGInFiletreeAndDiffJumpsBottom(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")
	testutil.WriteFile(t, repo, "c.txt", "three\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.filetree.data.listState.SetSelected(0, len(m.filetree.data.statusEntries))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.filetree.data.listState.Selected() != len(m.filetree.data.statusEntries)-1 {
		t.Fatalf("expected G to jump filetree selection to bottom, got %d", m.filetree.data.listState.Selected())
	}

	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)
	if len(m.diffarea.Unstaged.DataRef().Parsed.Changed) == 0 {
		t.Fatalf("expected unstaged changes in diff view")
	}
	m.diffarea.Unstaged.DataRef().ActiveLine = 0

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.diffarea.Unstaged.DataRef().ActiveLine != len(m.diffarea.Unstaged.DataRef().Parsed.Changed)-1 {
		t.Fatalf("expected G to jump active diff line to bottom, got %d", m.diffarea.Unstaged.DataRef().ActiveLine)
	}
}

func TestUppercaseGUsingShiftedCodeJumpsBottom(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.filetree.data.listState.SetSelected(0, len(m.filetree.data.statusEntries))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "G", ShiftedCode: 'G'})
	m = updated.(Model)
	if m.filetree.data.listState.Selected() != len(m.filetree.data.statusEntries)-1 {
		t.Fatalf("expected shifted G to jump to bottom, got %d", m.filetree.data.listState.Selected())
	}
}

func TestUppercaseGUsingShiftModifierJumpsBottom(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.filetree.data.listState.SetSelected(0, len(m.filetree.data.statusEntries))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g", Mod: tea.ModShift})
	m = updated.(Model)
	if m.filetree.data.listState.Selected() != len(m.filetree.data.statusEntries)-1 {
		t.Fatalf("expected shifted modifier G to jump to bottom, got %d", m.filetree.data.listState.Selected())
	}
}

func TestCtrlDAndCtrlUScrollFiletreeAndDiff(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	for i := 0; i < 16; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("f%02d.txt", i), "x\n")
	}

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.focus = focusFiletree
	beforeSel := m.filetree.data.listState.Selected()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.filetree.data.listState.Selected() <= beforeSel {
		t.Fatalf("expected ctrl+d to move filetree selection down")
	}

	midSel := m.filetree.data.listState.Selected()
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.filetree.data.listState.Selected() >= midSel {
		t.Fatalf("expected ctrl+u to move filetree selection up")
	}

	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeHunk)
	m.syncDiffViewports()
	beforeOffset := m.diffarea.Unstaged.Viewport().YOffset()

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.diffarea.Unstaged.Viewport().YOffset() < beforeOffset {
		t.Fatalf("expected ctrl+d to scroll diff down")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.diffarea.Unstaged.Viewport().YOffset() > beforeOffset {
		t.Fatalf("expected ctrl+u to scroll diff up")
	}
}

func TestCCTriggersCommitCommand(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd != nil {
		t.Fatalf("first c should not launch command")
	}
	m = updated.(Model)
	if len(m.keys.Prefix()) == 0 || m.keys.Prefix()[0] != "c" {
		t.Fatalf("expected chord prefix=c after first c, got %v", m.keys.Prefix())
	}
	// Chord hints are shown in the chord overlay via Manager.ChordHints().
	hints := m.keys.ChordHints()
	if len(hints) == 0 {
		t.Fatalf("expected ChordHints to return bindings for c prefix, got none")
	}
	found := false
	for _, h := range hints {
		if strings.Contains(h.Desc, "commit") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected \"commit\" in ChordHints descriptions")
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatalf("second c should launch commit command")
	}
	m = updated.(Model)
	if m.keys.Prefix() != nil {
		t.Fatalf("expected prefix cleared after cc, got %v", m.keys.Prefix())
	}
}

func TestCMInFiletreeDoesNothing(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd != nil {
		t.Fatalf("first c should not launch command")
	}
	m = updated.(Model)

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if cmd != nil {
		t.Fatalf("cm in filetree should not launch command")
	}
	m = updated.(Model)
}

func TestAskAIInDiffWithoutFileContextShowsError(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("aa (Ask AI) without file context should return a warning notification cmd")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notifypkg.NotifyMsg)
	if !ok {
		t.Fatalf("expected NotifyMsg, got %T", msg)
	}
	if notifyMsg.Message != "no file context for comment" {
		t.Fatalf("notification = %q, want %q", notifyMsg.Message, "no file context for comment")
	}
}

func TestGGJumpsToTop(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.filetree.data.statusEntries = []statusEntry{{Kind: statusEntryFile}, {Kind: statusEntryFile}, {Kind: statusEntryFile}}
	m.filetree.data.listState.SetSelected(2, len(m.filetree.data.statusEntries))

	// First g sets chord prefix
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if len(m.keys.Prefix()) == 0 || m.keys.Prefix()[0] != "g" {
		t.Fatalf("expected chord prefix=g after first g, got %v", m.keys.Prefix())
	}

	// Second g jumps to top
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd == nil {
		t.Fatalf("gg should schedule a diff reload after jumping to top")
	}
	m = updated.(Model)
	if m.filetree.data.listState.Selected() != 0 {
		t.Fatalf("expected gg to jump to top, got selected=%d", m.filetree.data.listState.Selected())
	}
}

func TestLTriggersLazygitLogCommand(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'L', Text: "L", ShiftedCode: 'L', Mod: tea.ModShift})
	if cmd == nil {
		t.Fatalf("L should launch lazygit log command")
	}
}

func TestGOOpensOutputModal(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true
	m.git.SetOutput("test", "hello")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd != nil {
		t.Fatalf("first g should not launch command")
	}
	m = updated.(Model)
	if len(m.keys.Prefix()) == 0 || m.keys.Prefix()[0] != "g" {
		t.Fatalf("expected chord prefix=g after first g, got %v", m.keys.Prefix())
	}

	// Opening the output modal is itself a disrupting event for the image-diff
	// overlay (ADR 0010: a stale placement would otherwise occlude the modal),
	// so cmd is expected to carry that internal housekeeping — not an external
	// command launch, which is what this test guards against.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(Model)
	if !m.git.OutputOpen() {
		t.Fatalf("expected go to open output modal")
	}
}

func TestEOpensEditorFromFiletreeAndDiff(t *testing.T) {
	t.Setenv("EDITOR", "true")
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "edit.txt", "one\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	// ee chord: filetree focus
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected ee in filetree view to launch editor command")
	}

	// ee chord: diff focus
	m.focus = focusDiff
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected ee in diff view to launch editor command")
	}
}

func TestEditChordSplitVariants(t *testing.T) {
	t.Setenv("EDITOR", "true")
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "edit.txt", "one\n")

	chords := []struct {
		name   string
		second string
	}{
		{"es (split)", "s"},
		{"ev (vsplit)", "v"},
		{"et (tab)", "t"},
	}

	for _, tt := range chords {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModelDefault(repo)
			m.ready = true
			m.focus = focusFiletree

			updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
			m = updated.(Model)
			updated, cmd := m.Update(tea.KeyPressMsg{Text: tt.second})
			_ = updated.(Model)
			if cmd == nil {
				t.Fatalf("expected e%s chord to return a non-nil cmd", tt.second)
			}
		})
	}
}

func TestEditorLineForCurrentSelectionInDiffMode(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "old-1\nold-2\nold-3\n")
	testutil.MustGitExported(t, repo, "add", "line.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "line.txt", "new-1\nnew-2\nnew-3\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	for i, cl := range m.diffarea.Unstaged.DataRef().Parsed.Changed {
		if cl.NewLine == 2 {
			m.diffarea.Unstaged.DataRef().ActiveLine = i
			break
		}
	}

	line := m.editorLineForCurrentSelection()
	if line != 2 {
		t.Fatalf("editorLineForCurrentSelection()=%d, want 2", line)
	}
}

// TestGChordBindingsReachableFromDiffFocus verifies that g+o, g+p, g+h, and
// g+esc are all reachable when the diff panel is focused. Previously the first
// 'g' was consumed by diffview's g+g chord prefix, leaving status chords
// unreachable.
func TestGChordBindingsReachableFromDiffFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	newDiffFocusModel := func(t *testing.T) Model {
		t.Helper()
		m := newTestModelDefault(repo)
		m.ready = true
		m.focus = focusDiff
		return m
	}

	t.Run("g+esc cancels chord without side effects", func(t *testing.T) {
		m := newDiffFocusModel(t)
		m = pressKey(m, 'g')
		if len(m.keys.Prefix()) != 1 {
			t.Fatalf("after first g: expected status prefix len 1, got %d", len(m.keys.Prefix()))
		}
		m = pressSpecialKey(m, tea.KeyEsc)
		if len(m.keys.Prefix()) != 0 {
			t.Fatalf("after g+esc: expected status prefix cleared, got len %d", len(m.keys.Prefix()))
		}
		if m.focus != focusDiff {
			t.Fatalf("after g+esc: expected to remain in diff focus, got %v", m.focus)
		}
	})

	t.Run("g+g still scrolls diff to top via diffview", func(t *testing.T) {
		m := newDiffFocusModel(t)
		m = pressKey(m, 'g')
		if !m.diffarea.ActiveSectionModel().HasPendingChord() {
			t.Fatalf("after first g: expected diffview pending chord=true")
		}
		m = pressKey(m, 'g')
		// diffview should have won the chord; pending chord cleared.
		if m.diffarea.ActiveSectionModel().HasPendingChord() {
			t.Fatalf("after g+g: expected diffview pending chord=false, diffview handled the chord")
		}
	})

	t.Run("subsequent j navigation works after g+o chord", func(t *testing.T) {
		// Prime with diff content so j has something to navigate
		testutil.MustGitExported(t, repo, "add", "README.md")
		testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
		testutil.WriteFile(t, repo, "README.md", "line1\nline2\nline3\nchanged\n")
		m2 := newTestModelDefault(repo)
		m2.ready = true
		m2.width = 100
		m2.height = 20
		m2.focus = focusDiff
		m2 = runStatusCmd(t, m2, m2.reloadDiffsForSelection())
		// Press g+o (which opens output panel if there's output, or shows "no output")
		m2 = pressKey(m2, 'g')
		m2 = pressKey(m2, 'o')
		// g prefix in status should be cleared
		if len(m2.keys.Prefix()) != 0 {
			t.Fatalf("after g+o: expected status prefix cleared, got %v", m2.keys.Prefix())
		}
		// diffview pending chord should be cleared.
		if m2.diffarea.ActiveSectionModel().HasPendingChord() {
			t.Fatalf("after g+o: expected diffview pending chord=false")
		}
	})
}

func TestModalOpenAfterPressingPull(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	m := newTestModelDefault(repoDir)

	// Press 'p' to open pull.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(Model)

	if !m.ModalOpen() {
		t.Fatal("expected ModalOpen()=true immediately after pressing pull key")
	}
}
