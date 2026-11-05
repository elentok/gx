package cmd

import (
	"github.com/elentok/gx/cmd/serverapi"
	"github.com/elentok/gx/cmd/ticketsmigrate"
	"github.com/elentok/gx/ui/nav"

	"github.com/spf13/cobra"
)

func newTicketsCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tickets",
		Aliases: []string{"tk"},
		Short:   "open the tickets UI",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.tui.RunTab(nav.TabTickets)
		},
	}
	cmd.AddCommand(
		newTicketsValidateCmd(d),
		newTicketsShowCmd(d),
		newTicketsLogCmd(d),
		newTicketsSetCmd(d),
		ticketsmigrate.New(ticketsmigrate.Deps{Env: d.base()}),
		newTicketsRootCmd(d),
		newTicketsEpicsCmd(d),
		newTicketsSchemaCmd(),
		newTicketsAddCmd(d),
		serverapi.NewAskCmd(d.serverapiDeps()),
		serverapi.NewAnswerCmd(d.serverapiDeps(), false),
		newTicketsSectionCmd(d),
		newTicketsMoveCmd(d),
		newTicketsCloseCmd(d),
	)
	cmd.AddCommand(ticketsFlowCommands(d)...)
	return cmd
}

func newVersionCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the gx version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runVersion(d.stdout)
		},
	}
}
