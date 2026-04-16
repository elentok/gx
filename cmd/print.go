package cmd

import (
	"fmt"
	"io"

	"gx/ui"

	"charm.land/lipgloss/v2"
)

// printBadge prints a step heading in the stashify-badge style.
// It emits a blank line before the badge for visual separation.
// If nerd is true and the terminal supports it, nerdText is used (which may include icons);
// otherwise plainText is used.
func printBadge(w io.Writer, nerd bool, nerdText, plainText string) {
	text := plainText
	if nerd {
		text = nerdText
	}
	if isTerminalWriter(w) {
		badge := lipgloss.NewStyle().
			Background(lipgloss.Color("4")).
			Foreground(lipgloss.Color("0")).
			PaddingLeft(1).
			PaddingRight(1).
			Render(text)
		fmt.Fprintln(w, "\n"+badge)
	} else {
		fmt.Fprintln(w, "\n"+text)
	}
}

// printSuccess prints a blank line then a green checkmark success message.
func printSuccess(w io.Writer, text string) {
	if isTerminalWriter(w) {
		style := lipgloss.NewStyle().Foreground(ui.ColorGreen)
		fmt.Fprintln(w, "\n"+style.Render("✔ "+text))
	} else {
		fmt.Fprintln(w, "\n✔ "+text)
	}
}

// printError prints a blank line then a red ✘ error message.
func printError(w io.Writer, text string) {
	if isTerminalWriter(w) {
		style := lipgloss.NewStyle().Foreground(ui.ColorRed)
		fmt.Fprintln(w, "\n"+style.Render("✘ "+text))
	} else {
		fmt.Fprintln(w, "\n✘ "+text)
	}
}
