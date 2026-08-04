package components

import (
	"image/color"
	"strings"

	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// UpdateConfirm applies standard yes/no confirm key handling.
// Returns nextYes, decided, accepted, handled.
func UpdateConfirm(msg tea.KeyPressMsg, yes bool) (bool, bool, bool, bool) {
	switch msg.String() {
	case "left", "h":
		return true, false, false, true
	case "right", "l":
		return false, false, false, true
	case "y":
		return true, true, true, true
	case "n", "esc", "q":
		return false, true, false, true
	case "enter":
		return yes, true, yes, true
	default:
		return yes, false, false, false
	}
}

var ConfirmHint = ui.RenderInlineBindings(
	key.NewBinding(key.WithHelp("h/l or ←/→", "choose")),
	key.NewBinding(key.WithHelp("y/n", "quick select")),
	key.NewBinding(key.WithHelp("enter", "confirm")),
	key.NewBinding(key.WithHelp("esc", "cancel")),
)

func RenderConfirmChoices(yes bool, nerd bool) string {
	return "  " + ui.RenderButton("Yes", yes, nerd) + "   " + ui.RenderButton("No", !yes, nerd)
}

func RenderConfirmContent(prompt string, yes bool, nerd bool, items ...string) string {
	var sb strings.Builder
	sb.WriteString(prompt)
	if len(items) > 0 {
		sb.WriteString("\n")
		for _, item := range items {
			sb.WriteString("\n  • ")
			sb.WriteString(item)
		}
	}
	sb.WriteString("\n\n")
	sb.WriteString(RenderConfirmChoices(yes, nerd))
	return sb.String()
}

// ConfirmModalMinWidth and ConfirmModalMaxWidth bound every confirmation
// dialog's rendered width regardless of what a caller passes in: confirm.Model
// forwards the raw screen width, and without a cap here the dialog would scale
// to span the whole terminal instead of reading as a small centered box.
const (
	ConfirmModalMinWidth = 56
	ConfirmModalMaxWidth = 72
)

func RenderConfirmModal(prompt string, yes bool, borderColor, yesColor, noColor, subtleColor color.Color, width int, items ...string) string {
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Body:        RenderConfirmContent(prompt, yes, false, items...),
		Hint:        ConfirmHint,
		Width:       clampConfirmWidth(width),
		BorderColor: borderColor,
		HintColor:   subtleColor,
	})
}

func clampConfirmWidth(width int) int {
	if width <= 0 || width > ConfirmModalMaxWidth {
		return ConfirmModalMaxWidth
	}
	if width < ConfirmModalMinWidth {
		return ConfirmModalMinWidth
	}
	return width
}

// ConfirmButtonBounds locates the Yes/No buttons within a confirm modal's own
// rendered coordinate frame (as returned by RenderConfirmModal), so a mouse
// click can be hit-tested against them without re-deriving the modal's
// border/padding layout.
type ConfirmButtonBounds struct {
	Row          int
	YesX0, YesX1 int
	NoX0, NoX1   int
}

// LocateConfirmButtons computes where the Yes/No buttons land in the modal
// frame RenderConfirmModal produces for the same prompt/items. The row/column
// offsets mirror RenderModalFrame's layout (1-line top border, then a single
// default padding column) and RenderConfirmChoices' fixed "  Yes   No"
// spacing, so they're derived here rather than duplicated as constants.
func LocateConfirmButtons(prompt string, items ...string) ConfirmButtonBounds {
	body := RenderConfirmContent(prompt, true, false, items...)
	row := len(strings.Split(body, "\n")) // choices is body's last line; +1 for the top border

	const colOffset = 2 // border char + default padding column
	yesW := ansi.StringWidth(ui.RenderButton("Yes", true, false))
	noW := ansi.StringWidth(ui.RenderButton("No", false, false))

	yesX0 := colOffset + 2 // "  " prefix in RenderConfirmChoices
	yesX1 := yesX0 + yesW
	noX0 := yesX1 + 3 // "   " separator in RenderConfirmChoices
	noX1 := noX0 + noW

	return ConfirmButtonBounds{Row: row, YesX0: yesX0, YesX1: yesX1, NoX0: noX0, NoX1: noX1}
}

// HitTest returns "yes" or "no" for the button under x,y, or "" if neither
// contains it.
func (b ConfirmButtonBounds) HitTest(x, y int) string {
	if y != b.Row {
		return ""
	}
	if x >= b.YesX0 && x < b.YesX1 {
		return "yes"
	}
	if x >= b.NoX0 && x < b.NoX1 {
		return "no"
	}
	return ""
}
