package imagediff

import (
	"os"

	"github.com/elentok/gx/ui/kittygraphics"

	"golang.org/x/sys/unix"
)

// DefaultDetectCapability detects the host terminal's kitty-graphics capability
// using stdout's TIOCGWINSZ for the cell pixel size. It is the production
// detector both diff-panel hosts pass to NewOverlay; tests inject a fake.
func DefaultDetectCapability() kittygraphics.Capability {
	return kittygraphics.DetectSupport(os.Getenv, queryStdoutWinSize, noProbe)
}

// WriteToStdout writes raw kitty escape sequences to stdout, as a side effect
// outside bubbletea's render loop (ADR 0010). It is the production writer both
// hosts pass to NewOverlay.
func WriteToStdout(data []byte) {
	_, _ = os.Stdout.Write(data)
}

// queryStdoutWinSize reads the terminal's cell grid and pixel dimensions via
// TIOCGWINSZ on stdout, used for aspect-correct image scaling.
func queryStdoutWinSize() (kittygraphics.WinSize, bool) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return kittygraphics.WinSize{}, false
	}
	return kittygraphics.WinSize{
		Cols:        int(ws.Col),
		Rows:        int(ws.Row),
		PixelWidth:  int(ws.Xpixel),
		PixelHeight: int(ws.Ypixel),
	}, true
}

// noProbe never actually probes the terminal: bubbletea owns stdin's read loop,
// and writing a query then blocking for a response would race with (and corrupt)
// its input handling. Without a probe, a tmux-hosted kitty terminal is detected
// as unsupported — the documented graceful fallback for that setup.
func noProbe(string) (response string, ok bool) {
	return "", false
}
