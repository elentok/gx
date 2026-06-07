// Package kittygraphics encodes and detects support for the kitty terminal
// graphics protocol (https://sw.kovidgoyal.net/kitty/graphics-protocol/),
// used to render inline image-diff overlays in the status diff panel.
//
// All I/O (env vars, winsize queries, escape-sequence probes) is injected so
// detection and encoding can be unit-tested without a real terminal.
package kittygraphics

import (
	"strings"

	"github.com/elentok/gx/ui"
)

// WinSize is the result of a terminal window-size query (e.g. TIOCGWINSZ),
// reporting both the cell grid and the pixel dimensions of that grid.
type WinSize struct {
	Cols, Rows              int
	PixelWidth, PixelHeight int
}

// Capability describes what the host terminal supports for kitty-graphics
// rendering, as determined by DetectSupport.
type Capability struct {
	// Supported reports whether the host terminal understands the kitty
	// graphics protocol (directly, or as the host of a tmux session).
	Supported bool

	// PixelsPerCol and PixelsPerRow are the host terminal's cell size in
	// pixels, used for aspect-correct image scaling. Zero when unknown.
	PixelsPerCol float64
	PixelsPerRow float64

	// TmuxPassthrough reports whether placements must be wrapped in the tmux
	// DCS passthrough envelope to reach a kitty host terminal.
	TmuxPassthrough bool
}

// kittyProbeQuery is the kitty graphics protocol query action: "is the
// terminal a kitty-graphics-capable terminal?". A kitty terminal answers with
// a response containing kittyProbeOK.
const kittyProbeQuery = "\033_Gi=1,a=q\033\\"

// kittyProbeOK is the marker present in a kitty terminal's response to
// kittyProbeQuery (the full response is "\033_Gi=1;OK\033\\").
const kittyProbeOK = "_Gi=1;OK"

// DetectSupport detects the host terminal's kitty-graphics capability using
// the provided dependencies, mirroring how ui.DetectTerminalFrom is injected
// with an env-var getter:
//
//   - getenv reads environment variables ($KITTY_WINDOW_ID, $TMUX, ...).
//   - queryWinSize returns the terminal's cell grid and pixel dimensions
//     (e.g. via TIOCGWINSZ); ok is false when the query fails or isn't
//     applicable (e.g. not a TTY).
//   - probe writes an escape-sequence query to the terminal and returns its
//     raw response; ok is false when no response was read (e.g. timeout).
//     It is only consulted when env vars alone can't tell whether a tmux
//     host is kitty (tmux does not forward $KITTY_* to its sessions).
func DetectSupport(
	getenv func(string) string,
	queryWinSize func() (WinSize, bool),
	probe func(query string) (response string, ok bool),
) Capability {
	term := ui.DetectTerminalFrom(getenv)

	hostIsKitty := term == ui.TerminalKitty || term == ui.TerminalKittyRemote
	inTmux := term == ui.TerminalTmux

	if inTmux && !hostIsKitty {
		if response, ok := probe(wrapTmuxProbe(kittyProbeQuery)); ok {
			hostIsKitty = strings.Contains(response, kittyProbeOK)
		}
	}

	capability := Capability{
		Supported:       hostIsKitty,
		TmuxPassthrough: inTmux && hostIsKitty,
	}

	if capability.Supported {
		if size, ok := queryWinSize(); ok && size.Cols > 0 && size.Rows > 0 {
			capability.PixelsPerCol = float64(size.PixelWidth) / float64(size.Cols)
			capability.PixelsPerRow = float64(size.PixelHeight) / float64(size.Rows)
		}
	}

	return capability
}

// wrapTmuxProbe wraps a probe query in the tmux DCS passthrough envelope so
// it reaches the host terminal when gx is running inside tmux.
func wrapTmuxProbe(query string) string {
	return string(wrapTmuxPassthrough([]byte(query)))
}
