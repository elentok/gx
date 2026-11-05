package cmd

import (
	"github.com/elentok/gx/cmd/serverapi"
	"github.com/elentok/gx/cmd/tickets"
	"github.com/elentok/gx/cmd/ticketsflow"
	"github.com/elentok/gx/cmd/ticketsmigrate"

	"github.com/spf13/cobra"
)

// ticketsDeps is the `gx tickets` hooks joined with the shared env.
func (d Deps) ticketsDeps() tickets.Deps {
	return tickets.Deps{
		Env:     d.Env,
		OpenTab: d.tui.RunTab,
	}
}

func newTicketsCmd(d Deps) *cobra.Command {
	extra := []*cobra.Command{
		ticketsmigrate.New(ticketsmigrate.Deps{Env: d.Env}),
		serverapi.NewAskCmd(d.serverapiDeps()),
		serverapi.NewAnswerCmd(d.serverapiDeps(), false),
	}
	flow := ticketsflow.Commands(ticketsflow.Deps{
		Env:         d.Env,
		CommandTree: func() *cobra.Command { return newRootCmd(d) },
	})
	return tickets.New(d.ticketsDeps(), append(extra, flow...)...)
}

func newVersionCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the gx version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runVersion(d.Stdout)
		},
	}
}
