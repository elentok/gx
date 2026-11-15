package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// splitBacklog splits "project:backlog".
func splitBacklog(arg string) (project, backlog string, err error) {
	project, backlog, ok := strings.Cut(arg, ":")
	if !ok || project == "" || backlog == "" {
		return "", "", fmt.Errorf("want <project:backlog>, got %q", arg)
	}
	return project, backlog, nil
}

func newAgentsStartDesignCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var hint string
	cmd := &cobra.Command{
		Use:   "start-design <project:backlog>",
		Short: "start a chat that proposes a Design theme for a backlog (returns the live one if there is one)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			project, backlog, err := splitBacklog(args[0])
			if err != nil {
				return err
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Sessions.StartDesign(ctxOrBackground(c.Context()), wire.StartDesignRequest{Project: project, Backlog: backlog, Hint: hint})
			return printSessionResult(c.OutOrStdout(), jsonOut, res, err)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.Flags().StringVar(&hint, "hint", "", "steer the theme the chat proposes")
	return cmd
}

// runAgentsFinishMove prints the map's address.
func runAgentsFinishMove(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req wire.FinishMoveRequest) error {
	res, err := cl.Sessions.FinishMove(ctxOrBackground(ctx), req)
	if err != nil {
		return mapServerErr(err)
	}
	if jsonOut {
		stamped, err := cmdbase.StampProvenance(res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
		if err != nil {
			return err
		}
		if err := writeJSON(w, stamped); err != nil {
			return err
		}
		return cmdbase.JSONRefusalExit(res.Refused)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintln(w, res.Address)
	return err
}

func newAgentsFinishMoveCmd(d Deps) *cobra.Command {
	var mapName string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "finish-move <project:backlog>",
		Short: "carry on an unfinished move of a backlog's tickets into a charting map",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			project, backlog, err := splitBacklog(args[0])
			if err != nil {
				return err
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runAgentsFinishMove(c.Context(), cl, c.OutOrStdout(), jsonOut, wire.FinishMoveRequest{Project: project, Backlog: backlog, Map: mapName})
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.Flags().StringVar(&mapName, "map", "", "name of the unfinished map")
	_ = cmd.MarkFlagRequired("map")
	return cmd
}
