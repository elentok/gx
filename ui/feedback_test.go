package ui

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
)

func TestJoinStatusSkipsEmptyParts(t *testing.T) {
	got := JoinStatus("pull complete", "", "view output")
	if got != "pull complete  ·  view output" {
		t.Fatalf("JoinStatus() = %q", got)
	}
}

func TestStatusWithHintsRendersInlineBindingHints(t *testing.T) {
	got := ansi.Strip(StatusWithHints("push complete", key.NewBinding(key.WithHelp("oo", "view output"))))
	if got != "push complete  ·  oo view output" {
		t.Fatalf("StatusWithHints() = %q", got)
	}
}
