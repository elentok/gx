package cmd

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestWarnOnScratchFoldFailure(t *testing.T) {
	noConfirm := func(string) (bool, error) { return true, nil }

	t.Run("fold succeeds: no warning", func(t *testing.T) {
		repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
		strayEpic := filepath.Join(repoDir, "feature-a", ".scratch", "bugs-01")
		testutil.Mkdir(t, strayEpic)
		testutil.WriteFile(t, strayEpic, "map.md", "stray content")

		getwd := func() (string, error) { return repoDir, nil }
		var w bytes.Buffer
		warnOnScratchFoldFailure(&w, getwd, noConfirm)

		if w.String() != "" {
			t.Errorf("output = %q, want empty", w.String())
		}
	})

	t.Run("fold fails: warning written", func(t *testing.T) {
		repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")

		strayEpic := filepath.Join(repoDir, "feature-a", ".scratch", "bugs-01")
		testutil.Mkdir(t, strayEpic)
		testutil.WriteFile(t, strayEpic, "issue-a.md", "stray version")

		canonicalEpic := filepath.Join(repoDir, ".scratch", "bugs-01")
		testutil.Mkdir(t, canonicalEpic)
		testutil.WriteFile(t, canonicalEpic, "issue-a.md", "canonical version")

		getwd := func() (string, error) { return repoDir, nil }
		var w bytes.Buffer
		warnOnScratchFoldFailure(&w, getwd, noConfirm)

		got := w.String()
		if !strings.HasPrefix(got, "warning: failed to fold stray .scratch directories: ") {
			t.Errorf("output = %q, want prefix %q", got, "warning: failed to fold stray .scratch directories: ")
		}
	})

	t.Run("getwd resolves outside a git repo: no-op", func(t *testing.T) {
		dir := t.TempDir()
		getwd := func() (string, error) { return dir, nil }
		var w bytes.Buffer
		warnOnScratchFoldFailure(&w, getwd, noConfirm)

		if w.String() != "" {
			t.Errorf("output = %q, want empty", w.String())
		}
	})
}

func TestResolveScratchCollisionPromptsAndMerges(t *testing.T) {
	var shownPrompt string
	confirmFn := func(prompt string) (bool, error) {
		shownPrompt = prompt
		return true, nil
	}

	action := tickets.FoldAction{EpicSlug: "bugs-01", WorktreeName: "feature-a"}
	choice, err := resolveScratchCollision(confirmFn, action)
	if err != nil {
		t.Fatalf("resolveScratchCollision: %v", err)
	}
	if choice != tickets.ResolveMerge {
		t.Errorf("choice = %v, want ResolveMerge", choice)
	}
	if shownPrompt == "" {
		t.Error("expected a prompt to be shown")
	}
}

func TestResolveScratchCollisionPromptsAndAutoRenames(t *testing.T) {
	confirmFn := func(string) (bool, error) { return false, nil }

	action := tickets.FoldAction{EpicSlug: "bugs-01", WorktreeName: "feature-a"}
	choice, err := resolveScratchCollision(confirmFn, action)
	if err != nil {
		t.Fatalf("resolveScratchCollision: %v", err)
	}
	if choice != tickets.ResolveAutoRename {
		t.Errorf("choice = %v, want ResolveAutoRename", choice)
	}
}

func TestResolveScratchCollisionPropagatesConfirmError(t *testing.T) {
	wantErr := errors.New("boom")
	confirmFn := func(string) (bool, error) { return false, wantErr }

	_, err := resolveScratchCollision(confirmFn, tickets.FoldAction{EpicSlug: "bugs-01"})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}
