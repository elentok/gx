package ui

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderInlineBindingsRendersJoinedKeyDescriptions(t *testing.T) {
	out := RenderInlineBindings(
		key.NewBinding(key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithHelp("enter", "open")),
	)

	plain := ansi.Strip(out)
	want := "j/k move · enter open"
	if plain != want {
		t.Fatalf("RenderInlineBindings() = %q, want %q", plain, want)
	}
}

func TestRenderInlineBindingsSkipsEmptyBindings(t *testing.T) {
	out := RenderInlineBindings(
		key.NewBinding(),
		key.NewBinding(key.WithHelp("", "")),
	)

	if out != "" {
		t.Fatalf("RenderInlineBindings() = %q, want empty string", ansi.Strip(out))
	}
}

func TestRenderInlineBindingsHandlesKeyOnlyAndDescOnly(t *testing.T) {
	out := RenderInlineBindings(
		key.NewBinding(key.WithHelp("esc", "")),
		key.NewBinding(key.WithHelp("", "cancel")),
	)

	plain := ansi.Strip(out)
	want := "esc · cancel"
	if plain != want {
		t.Fatalf("RenderInlineBindings() = %q, want %q", plain, want)
	}
}

func TestRenderInlineBindingsOmitsEmptyBindingBetweenEntries(t *testing.T) {
	out := RenderInlineBindings(
		key.NewBinding(key.WithHelp("a", "apply")),
		key.NewBinding(),
		key.NewBinding(key.WithHelp("q", "quit")),
	)

	plain := ansi.Strip(out)
	want := "a apply · q quit"
	if plain != want {
		t.Fatalf("RenderInlineBindings() = %q, want %q", plain, want)
	}
}
