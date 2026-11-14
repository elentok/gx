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

// splitArea splits "project:area".
func splitArea(arg string) (project, area string, err error) {
	project, area, ok := strings.Cut(arg, ":")
	if !ok || project == "" || area == "" {
		return "", "", fmt.Errorf("want <project:area>, got %q", arg)
	}
	return project, area, nil
}

func newAgentsStartDesignCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var hint string
	cmd := &cobra.Command{
		Use:   "start-design <project:area>",
		Short: "start a chat that proposes a Design theme for an area (returns the live one if there is one)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			project, area, err := splitArea(args[0])
			if err != nil {
				return err
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Sessions.StartDesign(ctxOrBackground(c.Context()), wire.StartDesignRequest{Project: project, Area: area, Hint: hint})
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
		Use:   "finish-move <project:area>",
		Short: "carry on an unfinished move of an area's tickets into a charting map",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			project, area, err := splitArea(args[0])
			if err != nil {
				return err
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runAgentsFinishMove(c.Context(), cl, c.OutOrStdout(), jsonOut, wire.FinishMoveRequest{Project: project, Area: area, Map: mapName})
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.Flags().StringVar(&mapName, "map", "", "name of the unfinished map")
	_ = cmd.MarkFlagRequired("map")
	return cmd
}
