package status

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	notifypkg "github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

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

// sendStashStagedChord sends the "S" then "s" chord.
func sendStashStagedChord(m Model) (Model, tea.Cmd) {
	m, cmd := sendKey(m, 'S', "S")
	if cmd != nil {
		panic("S prefix unexpectedly produced a command")
	}
	return sendKey(m, 's', "s")
}

func expectNothingToStash(t *testing.T, m Model, cmd tea.Cmd) {
	t.Helper()
	if m.stash.IsOpen {
		t.Fatal("stash modal should not open")
	}
	if cmd == nil {
		t.Fatal("expected a notify cmd")
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

func TestStashAll_EmptyTreeShowsNoticeAndDoesNotOpen(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true

	m, cmd := sendStashAllChord(m)
	expectNothingToStash(t, m, cmd)
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
	expectNothingToStash(t, m, cmd)
}

func TestStashAll_OpensModalOnDirtyTree(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n") // tracked modification

	m := newTestModelDefault(repo)
	m.ready = true

	m, _ = sendStashAllChord(m)
	if !m.stash.IsOpen {
		t.Fatal("expected stash modal to open on a dirty tree")
	}
}

// Driving a real stash to completion through the status seam yields a follow-up
// cmd (notify.Success + refresh) and the actual stash gets created.
func TestStashAll_StashedResultNotifiesAndRefreshes(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true

	m, _ = sendStashAllChord(m)
	if !m.stash.IsOpen {
		t.Fatal("expected stash modal to open")
	}

	// Enter fires the stash cmd through the sub-model; the seam routes it.
	m, cmd := sendSpecialKey(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("expected a stash command")
	}

	// Enter batches the stash cmd with a spinner tick; flatten the batch and
	// route the (private) done-msg back through the seam.
	m, follow := drainBatch(m, cmd)
	if m.stash.IsOpen {
		t.Fatal("expected stash sub-model to close after success")
	}
	if follow == nil {
		t.Fatal("expected a follow-up cmd (notify + refresh) after stash succeeded")
	}
}

func TestStashStaged_NothingStagedShowsNotice(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	// Only unstaged modifications — nothing staged to stash.
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true

	m, cmd := sendStashStagedChord(m)
	if m.stash.IsOpen {
		t.Fatal("stash modal should not open when nothing staged")
	}
	if cmd == nil {
		t.Fatal("expected a notify cmd")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notifypkg.NotifyMsg)
	if !ok {
		t.Fatalf("expected NotifyMsg, got %T", msg)
	}
	if notifyMsg.Message != "nothing staged to stash" {
		t.Fatalf("notify = %q, want %q", notifyMsg.Message, "nothing staged to stash")
	}
}

func TestStashStaged_OpensStagedModalWhenStagedChangesExist(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "staged.txt", "staged\n")
	if err := git.StagePath(repo, "staged.txt"); err != nil {
		t.Fatalf("stage staged.txt: %v", err)
	}

	m := newTestModelDefault(repo)
	m.ready = true

	m, _ = sendStashStagedChord(m)
	if !m.stash.IsOpen {
		t.Fatal("expected stash modal to open")
	}
	if !m.stash.StagedOnly() {
		t.Fatal("expected stash sub-model to be in staged-only mode")
	}
}

// drainBatch runs cmd, flattening any tea.BatchMsg, and feeds every resulting
// message back through the model's Update. It returns the final model and the
// last non-nil follow-up cmd produced.
func drainBatch(m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	var follow tea.Cmd
	var walk func(c tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil {
			return
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, sub := range batch {
				walk(sub)
			}
			return
		}
		next, f := m.Update(msg)
		m = asModel(next)
		if f != nil {
			follow = f
		}
	}
	walk(cmd)
	return m, follow
}

func sendSpecialKey(m Model, code rune) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: code})
	return asModel(next), cmd
}
