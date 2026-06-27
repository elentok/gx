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

func TestStatusWithHints_NoHints(t *testing.T) {
	got := StatusWithHints("done")
	if got != "done" {
		t.Fatalf("StatusWithHints(no hints) = %q", got)
	}
}

func TestMessageHelpers(t *testing.T) {
	if got := MessageComplete("pull"); got != "pull complete" {
		t.Errorf("MessageComplete = %q", got)
	}
	if got := MessageAborted("push"); got != "push aborted" {
		t.Errorf("MessageAborted = %q", got)
	}
	if got := MessageNoOutput(); got == "" {
		t.Error("MessageNoOutput should be non-empty")
	}
}

func TestHintHelpers_NonEmpty(t *testing.T) {
	hints := []string{
		HintDismiss(),
		HintDismissAndScroll(),
		HintSubmitCancel(),
		HintChecklistConfirm(),
		HintCancelScroll(),
	}
	for i, h := range hints {
		if h == "" {
			t.Errorf("hint[%d] should be non-empty", i)
		}
	}
}
