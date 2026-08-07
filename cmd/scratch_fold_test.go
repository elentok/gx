package cmd

import (
	"errors"
	"testing"

	"github.com/elentok/gx/tickets"
)

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
