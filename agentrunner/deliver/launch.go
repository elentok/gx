package deliver

import (
	"cmp"
	"errors"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
)

// LaunchCaps are the retype window of a launch prompt. A fresh agent may not
// list a skill yet and draws the unknown-command marker; the command did not
// run, so typing it again cannot run it twice.
type LaunchCaps struct {
	// Marker is the pane text for an unknown command; empty is claude's.
	Marker string
	// Window is how long the marker may stay on screen, counted from its first
	// sighting, before the send fails with ErrUnknownSkill.
	Window time.Duration
	// RetypeEvery is the wait between retypes; Poll is the wait between reads.
	RetypeEvery, Poll time.Duration
}

// DefaultLaunchCaps retype for about 30 seconds.
var DefaultLaunchCaps = LaunchCaps{Window: 30 * time.Second, RetypeEvery: 3 * time.Second, Poll: 500 * time.Millisecond}

// retypable is a send result that leaves the text unsent, so a retype is safe.
func retypable(err error) bool {
	return err == nil || errors.Is(err, claudeui.ErrStartTimeout) || errors.Is(err, claudeui.ErrStuckSubmission)
}

// deliverLaunch sends once with the policy's shape, then types again while the
// pane shows the unknown-command marker, up to the window.
func deliverLaunch(t Input, text string, p Policy) error {
	lc := *p.Launch
	marker := cmp.Or(lc.Marker, claudeui.UnknownCommandMarker)
	send := func() error {
		if p.Slash != nil {
			return deliverSlash(t, text, *p.Slash, p.Report, true)
		}
		return deliverTurn(t, text, p)
	}
	err := send()
	var seen, sinceSend time.Duration
	for retypable(err) {
		rows, _, rerr := t.ReadRows()
		if rerr != nil {
			return rerr
		}
		if !screenHas(rows, marker) {
			return err
		}
		if seen >= lc.Window {
			return errUnknown(text, marker)
		}
		if started, werr := t.WaitStart(lc.Poll); werr != nil || started {
			return werr
		}
		seen += lc.Poll
		sinceSend += lc.Poll
		if sinceSend >= lc.RetypeEvery {
			if p.Report != nil {
				p.Report(claudeui.NudgePrompt)
			}
			sinceSend = 0
			err = send()
		}
	}
	return err
}
