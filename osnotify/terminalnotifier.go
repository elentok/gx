package osnotify

import "context"

type terminalNotifier struct {
	path string
	run  Runner
}

func newTerminalNotifier(path string, run Runner) terminalNotifier {
	return terminalNotifier{path: path, run: run}
}

func (n terminalNotifier) Notify(ctx context.Context, note Notification) error {
	args := []string{"-title", note.Title, "-message", note.Body}
	link := note.Link
	if note.Execute != "" {
		link = ""
	}
	for _, opt := range []struct{ flag, value string }{
		{"-group", note.Group},
		{"-execute", note.Execute},
		{"-open", link},
		{"-activate", note.Activate},
	} {
		if opt.value != "" {
			args = append(args, opt.flag, opt.value)
		}
	}
	return runWithTimeout(ctx, n.run, n.path, args...)
}
