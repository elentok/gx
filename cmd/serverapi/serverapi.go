// Package serverapi holds the `gx server` verbs that only call the server's
// API: reads, queue and ticket writes, web login admin and the repair verbs.
// They need wire types and apiclient, never the runtime server package.
package serverapi

import (
	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
)

// Deps is what the API verbs need: the shared env and whether stdin is a
// terminal (the web password prompt reads it).
type Deps struct {
	cmdbase.Env
	StdinIsTerminal func() bool
}

// Commands returns the children of `gx server` that are API verbs. Root's
// server command registers them next to the process-control verbs.
func Commands(d Deps) []*cobra.Command {
	return []*cobra.Command{
		newServerSnapshotCmd(d), newServerEventsCmd(d), newServerLocksCmd(d), newServerTicketsCmd(d),
		newServerEpicsCmd(d), newServerStatsCmd(d), newServerIterationsCmd(d), newServerAgentsCmd(d),
		newServerQueueCmd(d), newServerOneOffCmd(d), newServerReloadConfigCmd(d), newServerConfigStatusCmd(d),
		newServerChecksCmd(d), newServerChecksSaveCmd(d), newServerDefaultHarnessCmd(d), newServerDefaultHarnessSetCmd(d),
		newServerDiskCmd(d), newServerHealthCmd(d), newServerRestartCheckCmd(d), newServerRestartNowCmd(d),
		newServerCPUSpikeCmd(d), newServerWebURLCmd(d), newServerWebNewRootCmd(d), newServerWebResetCmd(d),
		newServerWebResetAuthenticatorCmd(d), newServerWebLoginsCmd(d), newServerWebRevokeCmd(d), newServerWebPasswordCmd(d),
	}
}
