package nativerunner

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
)

// Died maps err from a holder call for the session label, as Pty does.
func Died(p *Pty, label string, err error) error {
	return p.died(&ptySession{session: agentrunner.Session{Label: label}}, err)
}

// DeliverErr maps err from a Deliver call for the session label, as the launch
// relay does.
func DeliverErr(p *Pty, label string, err error) error {
	pr := &ptyRelay{p: p, ss: &ptySession{session: agentrunner.Session{Label: label}}}
	return pr.deliverErr(err)
}

// LaunchCommand is the command p runs the agent for opts with.
func LaunchCommand(p *Pty, opts agentrunner.StartOptions, dir, sessionID string) (harness.Command, error) {
	return p.launcher().launchCommand(opts, dir, sessionID)
}

// KillLeftovers runs the stop path's leftover kill for a session whose claude
// led process group pgid.
func KillLeftovers(p *Pty, pgid int) {
	p.killLeftovers(&ptySession{claudePID: pgid})
}
