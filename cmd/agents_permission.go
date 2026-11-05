package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
)

func newAgentsPermissionCmd(d deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "permission <session>",
		Short: "show the permission dialog a blocked session waits on",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			res, err := cl.Sessions.Permission(c.Context(), args[0])
			if err != nil {
				return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
			}
			if jsonOut {
				return cmdbase.EncodeProvenance(c.OutOrStdout(), res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "%s\t%s\n%s\n", res.Kind, res.RequestID, res.Text)
			return err
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}
