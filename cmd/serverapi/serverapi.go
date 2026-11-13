// Package serverapi holds the `gx server` verbs that only call the server's
// API: reads, queue and ticket writes, web login admin and the repair verbs.
// They need wire types and apiclient, never the runtime server package.
package serverapi

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/serverapi/oneoff"
	"github.com/elentok/gx/server/wire"
)

// notRunningMessage is the refusal every write gets when no server answers.
const notRunningMessage = "no server is running; start it with `gx server start`"

// runTargets dials, makes a many-target write and prints it (see cmdbase.RunTargets).
func runTargets(c *cobra.Command, d Deps, jsonOut bool, targets []string,
	call func(context.Context, *apiclient.Client) (wire.TargetsResult, error),
	one func(io.Writer, wire.TargetResult) error,
) error {
	cl, err := d.Dial()()
	if err != nil {
		return err
	}
	return cmdbase.RunTargets(c.Context(), c.OutOrStdout(), jsonOut, targets, notRunningMessage,
		func(ctx context.Context) (wire.TargetsResult, error) { return call(ctx, cl) },
		func(err error) error { return fmt.Errorf("server write failed: %w", err) },
		one)
}

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
		newServerEpicsCmd(d), newServerStatsCmd(d), newServerTipsCmd(d), newServerIterationsCmd(d), newServerAgentsCmd(d),
		newServerQueueCmd(d), oneoff.NewCmd(d.Env), newServerReloadConfigCmd(d), newServerConfigStatusCmd(d),
		newServerChecksCmd(d), newServerChecksSaveCmd(d), newServerDefaultHarnessCmd(d), newServerDefaultHarnessSetCmd(d), newServerSettingsCmd(d), newServerSettingsSetCmd(d),
		newServerCleanupCmd(d), newServerCleanupRemoveCmd(d), newServerCleanupRemoveUnmergedCmd(d), newServerCleanupArchiveCmd(d), newServerCleanupAgentCmd(d), newServerCleanupDeleteCmd(d), newServerDiskCmd(d), newServerHealthCmd(d), newServerRestartCheckCmd(d), newServerRestartNowCmd(d),
		newServerCPUSpikeCmd(d), newServerWebURLCmd(d), newServerWebNewRootCmd(d), newServerWebResetCmd(d),
		newServerWebResetAuthenticatorCmd(d), newServerWebLoginsCmd(d), newServerWebRevokeCmd(d), newServerWebPasswordCmd(d),
		newServerWorkspacesCmd(d),
		newServerHoldCmd(d),
		newServerUnholdCmd(d),
		newServerTriageCmd(d),
	}
}
