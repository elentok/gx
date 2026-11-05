package cmd

import (
	"github.com/elentok/gx/cmd/ticketref"

	"github.com/spf13/cobra"
)

func newTicketsValidateCmd(d deps) *cobra.Command {
	var validateAll bool
	cmd := &cobra.Command{
		Use:   "validate <addr|path> | --all",
		Short: "validate a ticket and its whole project, by address or file path; --all walks the store",
		Args: func(_ *cobra.Command, args []string) error {
			if validateAll {
				return cobra.NoArgs(nil, args)
			}
			return cobra.ExactArgs(1)(nil, args)
		},
		RunE: func(c *cobra.Command, args []string) error {
			if validateAll {
				cfg, err := d.dirs().Load()
				if err != nil {
					return err
				}
				return runTicketsValidateAll(cfg.TicketStore.Path, c.OutOrStdout(), c.ErrOrStderr())
			}
			path, err := ticketref.ResolveTicketRef(d.dirs(), d.getwd, args[0])
			if err != nil {
				return err
			}
			return runTicketsValidate(path, c.OutOrStdout(), c.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&validateAll, "all", false, "validate every project in the ticket store")
	return cmd
}

func newTicketsShowCmd(d deps) *cobra.Command {
	var showJSON bool
	var showSection string
	cmd := &cobra.Command{
		Use: "show <addr>",
		// Agents keep guessing `gx tickets list <epic>`.
		Aliases: []string{"list"},
		Short:   "print a ticket (project:epic/06, epic/06 or 06) or an epic and its tickets (project:epic or epic)",
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsShow(d.dirs(), cwd, args[0], showJSON, showSection, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&showJSON, "json", false, "emit structured JSON instead of text")
	cmd.Flags().StringVar(&showSection, "section", "", "print only this ## section, from the ticket or from every ticket of an epic that has it")
	cmd.MarkFlagsMutuallyExclusive("json", "section")
	return cmd
}

func newTicketsRootCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "root",
		Short: "print the ticket root (in the ticket store, or .scratch before migrate) for the current repo",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsRoot(d.dirs(), cwd, c.OutOrStdout())
		},
	}
}

func newTicketsEpicsCmd(d deps) *cobra.Command {
	var mapsOnly bool
	cmd := &cobra.Command{
		Use:   "epics",
		Short: "print bare epic slugs under the current repo's ticket root",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsEpics(d.dirs(), cwd, c.OutOrStdout(), mapsOnly)
		},
	}
	cmd.Flags().BoolVar(&mapsOnly, "maps", false, "only print epics with ticket.md kind: map")
	return cmd
}
