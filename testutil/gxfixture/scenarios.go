package main

import (
	"net"

	"github.com/elentok/gx/testutil/gxfixture/limitseed"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

const project = limitseed.Project

// The scenario data model lives in the spec package; the aliases keep this
// package's code short.
type (
	ticket         = spec.Ticket
	epic           = spec.Epic
	scenario       = spec.Scenario
	archivedEpic   = spec.ArchivedEpic
	recommendation = spec.Recommendation
	sessionSpec    = spec.SessionSpec
	commandStep    = spec.CommandStep
	restartSpec    = spec.RestartSpec
	webMode        = spec.WebMode
)

const (
	webSignedIn     = spec.WebSignedIn
	webSetupPending = spec.WebSetupPending
	webSignedOut    = spec.WebSignedOut
)

// freePort asks the kernel for an unused loopback port.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
