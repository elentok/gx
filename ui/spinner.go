package ui

import "charm.land/bubbles/v2/spinner"

// ProgressSpinner is the circle-slice pie-fill spinner Tickets/Queue and
// Agents use for in-progress work. Frames fill 1/8 to full then empty back
// down (rather than cutting straight from full back to 1/8) so the loop point
// has no visible jump.
var ProgressSpinner = spinner.Spinner{
	Frames: []string{
		"\U000F0A9E", "\U000F0A9F", "\U000F0AA0", "\U000F0AA1",
		"\U000F0AA2", "\U000F0AA3", "\U000F0AA4", "\U000F0AA5",
		"\U000F0AA4", "\U000F0AA3", "\U000F0AA2", "\U000F0AA1",
		"\U000F0AA0", "\U000F0A9F",
	},
	FPS: spinner.Dot.FPS,
}

// ASCIIProgressSpinner stands in for ProgressSpinner without a nerd font.
var ASCIIProgressSpinner = spinner.Spinner{
	Frames: []string{"-", "\\", "|", "/"},
	FPS:    spinner.Dot.FPS,
}

// ProgressSpinnerFor picks the spinner matching the icon set.
func ProgressSpinnerFor(useNerdFont bool) spinner.Spinner {
	if useNerdFont {
		return ProgressSpinner
	}
	return ASCIIProgressSpinner
}
