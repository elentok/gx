package osnotify

import "context"

// osascriptSource reads title and body from argv instead of having them spliced
// into the script, so a quote in the body cannot break or inject AppleScript.
const osascriptSource = `on run argv
display notification (item 2 of argv) with title (item 1 of argv)
end run`

type osascript struct {
	run Runner
}

func newOSAScript(run Runner) osascript {
	return osascript{run: run}
}

// Notify ignores Group, Execute and Activate: osascript has no click action,
// so a Link goes in the body.
func (n osascript) Notify(ctx context.Context, note Notification) error {
	body := note.Body
	if note.Link != "" {
		body += "\n" + note.Link
	}
	return runWithTimeout(ctx, n.run, "osascript", "-e", osascriptSource, note.Title, body)
}
