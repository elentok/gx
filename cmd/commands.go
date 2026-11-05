package cmd

import (
	"io"

	"github.com/elentok/gx/cmd/serverapi"
	"github.com/elentok/gx/cmd/tickets"
	"github.com/elentok/gx/cmd/ticketsmigrate"
	"github.com/elentok/gx/config"

	"github.com/spf13/cobra"
)

// ticketsDeps is the `gx tickets` hooks joined with the shared env.
func (d deps) ticketsDeps() tickets.Deps {
	return tickets.Deps{
		Env:     d.base(),
		OpenTab: d.tui.RunTab,
		PrintSection: func(dirs config.Dirs, cwd, addr, section string, w io.Writer) error {
			return runTicketsShow(dirs, cwd, addr, false, section, w)
		},
	}
}

func newTicketsCmd(d deps) *cobra.Command {
	extra := []*cobra.Command{
		newTicketsValidateCmd(d),
		newTicketsShowCmd(d),
		newTicketsLogCmd(d),
		ticketsmigrate.New(ticketsmigrate.Deps{Env: d.base()}),
		newTicketsRootCmd(d),
		newTicketsEpicsCmd(d),
		serverapi.NewAskCmd(d.serverapiDeps()),
		serverapi.NewAnswerCmd(d.serverapiDeps(), false),
	}
	return tickets.New(d.ticketsDeps(), append(extra, ticketsFlowCommands(d)...)...)
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
