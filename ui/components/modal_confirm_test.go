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

func TestLocateConfirmButtonsMatchesRenderedModal(t *testing.T) {
	prompt := "Delete this ticket?"
	r := RenderConfirmModal(
		prompt,
		true,
		lipgloss.Color("240"),
		lipgloss.Color("2"),
		lipgloss.Color("1"),
		lipgloss.Color("8"),
		60,
	)
	lines := strings.Split(r, "\n")
	bounds := LocateConfirmButtons(prompt)
	if bounds.Row < 0 || bounds.Row >= len(lines) {
		t.Fatalf("bounds.Row=%d out of range (0..%d)", bounds.Row, len(lines)-1)
	}
	row := lines[bounds.Row]

	yes := strings.TrimSpace(ansi.Strip(ansi.Cut(row, bounds.YesX0, bounds.YesX1)))
	if yes != "Yes" {
		t.Fatalf("expected Yes at [%d,%d) on row %d, got %q: %q", bounds.YesX0, bounds.YesX1, bounds.Row, yes, ansi.Strip(row))
	}
	no := strings.TrimSpace(ansi.Strip(ansi.Cut(row, bounds.NoX0, bounds.NoX1)))
	if no != "No" {
		t.Fatalf("expected No at [%d,%d) on row %d, got %q: %q", bounds.NoX0, bounds.NoX1, bounds.Row, no, ansi.Strip(row))
	}

	if got := bounds.HitTest(bounds.YesX0, bounds.Row); got != "yes" {
		t.Errorf("HitTest on Yes bounds = %q, want yes", got)
	}
	if got := bounds.HitTest(bounds.NoX0, bounds.Row); got != "no" {
		t.Errorf("HitTest on No bounds = %q, want no", got)
	}
	if got := bounds.HitTest(0, 0); got != "" {
		t.Errorf("HitTest outside buttons = %q, want empty", got)
	}
}

func TestRenderConfirmModalCapsWidthRegardlessOfScreenWidth(t *testing.T) {
	r := RenderConfirmModal(
		"Prompt?",
		true,
		lipgloss.Color("240"),
		lipgloss.Color("2"),
		lipgloss.Color("1"),
		lipgloss.Color("8"),
		300,
	)
	lines := strings.Split(r, "\n")
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > ConfirmModalMaxWidth {
			t.Fatalf("line %d width = %d, want <= %d: %q", i, got, ConfirmModalMaxWidth, line)
		}
	}
}
