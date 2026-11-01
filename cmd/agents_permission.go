package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
			res, err := cl.SessionPermission(c.Context(), args[0])
			if err != nil {
				return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
			}
			if jsonOut {
				return encodeProvenance(c.OutOrStdout(), res, viaServer, callerActor(os.Getwd))
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "%s\t%s\n%s\n", res.Kind, res.RequestID, res.Text)
			return err
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}
