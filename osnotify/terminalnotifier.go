package osnotify

import "context"

type terminalNotifier struct {
	path string
	run  Runner
}

func newTerminalNotifier(path string, run Runner) terminalNotifier {
	return terminalNotifier{path: path, run: run}
}

// Notify uses a fixed group so a new banner replaces the previous one.
func (n terminalNotifier) Notify(ctx context.Context, title, body string) error {
	return runWithTimeout(ctx, n.run, n.path, "-title", title, "-message", body, "-group", "gx-config")
}
