package components

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestUpdateConfirmKeyHandling(t *testing.T) {
	if next, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: 'h', Text: "h"}, false); !handled || decided || accepted || !next {
		t.Fatalf("left/h should set yes without deciding")
	}

	if next, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: 'l', Text: "l"}, true); !handled || decided || accepted || next {
		t.Fatalf("right/l should set no without deciding")
	}

	if _, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: 'y', Text: "y"}, false); !handled || !decided || !accepted {
		t.Fatalf("y should accept")
	}

	if _, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: 'n', Text: "n"}, true); !handled || !decided || accepted {
		t.Fatalf("n should reject")
	}

	if _, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter}, true); !handled || !decided || !accepted {
		t.Fatalf("enter should accept when yes selected")
	}

	if _, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter}, false); !handled || !decided || accepted {
		t.Fatalf("enter should reject when no selected")
	}
}

func TestRenderSteps(t *testing.T) {
	steps := []Step{
		{TitleBefore: "fetch", TitleAfter: "fetched", TitleFailed: "fetch failed", RunningTitle: "fetching..."},
		{TitleBefore: "push", IsDone: true, TitleAfter: "pushed"},
		{TitleBefore: "rebase", HasFailed: true, TitleFailed: "rebase failed"},
		{TitleBefore: "stash", IsRunning: true, RunningTitle: "stashing..."},
	}
	rendered := RenderSteps(steps, ">")
	plain := ansi.Strip(rendered)
	for _, want := range []string{"fetch", "pushed", "rebase failed", "stashing..."} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected %q in RenderSteps output: %q", want, plain)
		}
	}
}

func TestRenderOutputModal_NonEmpty(t *testing.T) {
	out := RenderOutputModal("Title", "body content", "hint", color.White, color.Black, color.RGBA{R: 128, G: 128, B: 128, A: 255}, 40)
	if out == "" {
		t.Error("expected non-empty RenderOutputModal")
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "body content") {
		t.Errorf("expected body in output modal, got: %q", plain)
	}
}

func TestRenderInputModal_NonEmpty(t *testing.T) {
	out := RenderInputModal("Input", "Enter value:", "> cursor", "hint", color.White, color.Black, color.RGBA{R: 128, G: 128, B: 128, A: 255}, 40)
	if out == "" {
		t.Error("expected non-empty RenderInputModal")
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Enter value:") {
		t.Errorf("expected prompt in input modal, got: %q", plain)
	}
}

func TestRenderConfirmModalIncludesPrompt(t *testing.T) {
	r := RenderConfirmModal(
		"Prompt?",
		true,
		lipgloss.Color("240"),
		lipgloss.Color("2"),
		lipgloss.Color("1"),
		lipgloss.Color("8"),
		40,
	)
	if r == "" {
		t.Fatalf("expected non-empty rendered modal")
	}
	plain := ansi.Strip(r)
	for _, want := range []string{"Prompt?", "Yes", "No", "choose", "quick select"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected %q in confirm modal: %q", want, plain)
		}
	}
}
