package cmd

import (
	"github.com/elentok/gx/cmd/serverapi"
	"github.com/elentok/gx/cmd/tickets"
	"github.com/elentok/gx/cmd/ticketsflow"
	"github.com/elentok/gx/cmd/ticketsmigrate"

	"github.com/spf13/cobra"
)

// ticketsDeps is the `gx tickets` hooks joined with the shared env.
func (d deps) ticketsDeps() tickets.Deps {
	return tickets.Deps{
		Env:     d.base(),
		OpenTab: d.tui.RunTab,
	}
}

func newTicketsCmd(d deps) *cobra.Command {
	extra := []*cobra.Command{
		ticketsmigrate.New(ticketsmigrate.Deps{Env: d.base()}),
		serverapi.NewAskCmd(d.serverapiDeps()),
		serverapi.NewAnswerCmd(d.serverapiDeps(), false),
	}
	flow := ticketsflow.Commands(ticketsflow.Deps{
		Env:         d.base(),
		CommandTree: func() *cobra.Command { return newRootCmd(d) },
	})
	return tickets.New(d.ticketsDeps(), append(extra, flow...)...)
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
