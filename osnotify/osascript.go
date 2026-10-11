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

func (n osascript) Notify(ctx context.Context, title, body string) error {
	return runWithTimeout(ctx, n.run, "osascript", "-e", osascriptSource, title, body)
}
