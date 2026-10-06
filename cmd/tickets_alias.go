package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	groupTicketContent = "ticket-content"
	groupServer        = "server"
)

// deprecatedRepairAlias hides an old `gx tickets <verb>` repair command and
// makes it warn on stderr; it keeps working until cutover to `gx server tickets`.
func deprecatedRepairAlias(c *cobra.Command) *cobra.Command {
	c.Hidden = true
	run := c.RunE
	c.RunE = func(c *cobra.Command, args []string) error {
		fmt.Fprintf(c.ErrOrStderr(), "warning: `gx tickets %s` is deprecated; use `gx server tickets %s`\n", c.Name(), c.Name())
		return run(c, args)
	}
	return c
}
