package osnotify

import "context"

type notifySend struct {
	path string
	run  Runner
}

func newNotifySend(path string, run Runner) notifySend {
	return notifySend{path: path, run: run}
}

// Notify ignores Group, Execute and Activate: notify-send has no click action,
// so a Link goes in the body. The "--" keeps a title starting with "-" from
// being read as a flag.
func (n notifySend) Notify(ctx context.Context, note Notification) error {
	body := note.Body
	if note.Link != "" {
		body += "\n" + note.Link
	}
	return runWithTimeout(ctx, n.run, n.path, "--", note.Title, body)
}
