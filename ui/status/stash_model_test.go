package status

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	notifypkg "github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

func stashListOutput(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "stash", "list")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git stash list: %v\n%s", err, out)
	}
	return string(out)
}

func asModel(next tea.Model) Model {
	if p, ok := next.(*Model); ok {
		return *p
	}
	return next.(Model)
}

func sendKey(m Model, code rune, text string) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: code, Text: text})
	return asModel(next), cmd
}

func sendSpecialKey(m Model, code rune) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: code})
	return asModel(next), cmd
}

func typeString(m Model, s string) Model {
	for _, r := range s {
		m, _ = sendKey(m, r, string(r))
	}
	return m
}

// sendStashAllChord sends the "S" then "a" chord and returns the resulting
// model plus the cmd produced by completing the chord.
func sendStashAllChord(m Model) (Model, tea.Cmd) {
	m, cmd := sendKey(m, 'S', "S")
	if cmd != nil {
		// "S" alone only starts a chord prefix; it must not fire a command.
		panic("S prefix unexpectedly produced a command")
	}
	return sendKey(m, 'a', "a")
}

func TestStashAll_EmptyTreeShowsNoticeAndDoesNotOpen(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true

	m, cmd := sendStashAllChord(m)
	if m.stashOpen {
		t.Fatal("stash modal should not open on a clean tree")
	}
	if cmd == nil {
		t.Fatal("expected a notify cmd on clean tree")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notifypkg.NotifyMsg)
	if !ok {
		t.Fatalf("expected NotifyMsg, got %T", msg)
	}
	if notifyMsg.Message != "nothing to stash" {
		t.Fatalf("notify = %q, want %q", notifyMsg.Message, "nothing to stash")
	}
}

func TestStashAll_UntrackedOnlyIsNothingToStash(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	// Untracked files are out of scope (no --include-untracked), so a tree of
	// only-untracked files must read as "nothing to stash".
	testutil.WriteFile(t, repo, "new.txt", "untracked\n")

	m := newTestModelDefault(repo)
	m.ready = true

	m, cmd := sendStashAllChord(m)
	if m.stashOpen {
		t.Fatal("stash modal should not open for untracked-only tree")
	}
	if cmd == nil {
		t.Fatal("expected a notify cmd for untracked-only tree")
	}
	if msg, ok := cmd().(notifypkg.NotifyMsg); !ok || msg.Message != "nothing to stash" {
		t.Fatalf("notify = %#v, want 'nothing to stash'", cmd())
	}
}

func TestStashAll_OpensModalOnDirtyTree(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n") // tracked modification

	m := newTestModelDefault(repo)
	m.ready = true

	m, cmd := sendStashAllChord(m)
	if !m.stashOpen {
		t.Fatal("expected stash modal to open on a dirty tree")
	}
	if m.stashStagedOnly {
		t.Fatal("stash-all variant must not set stagedOnly")
	}
	if cmd != nil {
		t.Fatal("opening the modal should not fire a command")
	}
}

func TestStashAll_EscCancelsWithoutStashing(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true

	m, _ = sendStashAllChord(m)
	if !m.stashOpen {
		t.Fatal("expected stash modal to open")
	}
	m, cmd := sendSpecialKey(m, tea.KeyEscape)
	if m.stashOpen {
		t.Fatal("Esc should close the stash modal")
	}
	if cmd != nil {
		t.Fatal("Esc should not fire a stash command")
	}
	if list := stashListOutput(t, repo); strings.TrimSpace(list) != "" {
		t.Fatalf("expected no stash after cancel, got: %q", list)
	}
}

func TestStashAll_SubmitStashesAndRefreshes(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true

	m, _ = sendStashAllChord(m)
	if !m.stashOpen {
		t.Fatal("expected stash modal to open")
	}
	m = typeString(m, "mystash")

	m, cmd := sendSpecialKey(m, tea.KeyEnter)
	if m.stashOpen {
		t.Fatal("Enter should close the stash modal")
	}
	if cmd == nil {
		t.Fatal("Enter should fire a stash command")
	}
	// Run the stash command and feed the resulting message back into the model.
	msg := cmd()
	finished, ok := msg.(stashFinishedMsg)
	if !ok {
		t.Fatalf("expected stashFinishedMsg, got %T", msg)
	}
	if finished.err != nil {
		t.Fatalf("stash failed: %v\n%s", finished.err, finished.output)
	}
	if _, followup := m.Update(finished); followup == nil {
		t.Fatal("expected a follow-up cmd (notify + refresh) after stash finished")
	}

	if list := stashListOutput(t, repo); !strings.Contains(list, "mystash") {
		t.Fatalf("expected a stash named mystash, got: %q", list)
	}
	// Working tree should be clean after stashing all changes.
	files, err := git.ListStageFiles(repo)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected clean tree after stash, got %d files", len(files))
	}
}
