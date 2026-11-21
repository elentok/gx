package serverapi

import (
	"context"
	"fmt"
	"io"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// newServerInitiativesCmd lists the Initiatives the Initiatives page shows.
func newServerInitiativesCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "initiatives",
		Short: "list each project's Initiatives with state, done counts and current Milestone",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.InitiativeList, error) {
					return cl.Reads.Initiatives(ctx)
				},
				printInitiatives)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

// printInitiatives writes one line per Initiative: address, state, done of
// total, then the current Milestone or "-".
func printInitiatives(w io.Writer, l wire.InitiativeList) error {
	for _, p := range l.Projects {
		for _, r := range p.Initiatives {
			state := r.State
			if r.State == "stopped" {
				state = fmt.Sprintf("stopped at Milestone %d gate", r.StoppedAt)
			}
			current := "-"
			if r.Current != "" {
				current = r.Current
			}
			if _, err := fmt.Fprintf(w, "%s\t%s\t%d of %d done\t%s\n", r.Address, state, r.Done, r.Total, current); err != nil {
				return err
			}
		}
	}
	return nil
}

// newServerInitiativeCmd shows one Initiative's page data.
func newServerInitiativeCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "initiative <address>",
		Short: "show one Initiative: Destination, Milestones, Assumptions, Deferred items, gate notes",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.InitiativePage, error) {
					return cl.Reads.InitiativeDetail(ctx, args[0])
				},
				printInitiative)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

// printInitiative writes the header, one line per Milestone and the verbatim
// unparsed lines.
func printInitiative(w io.Writer, p wire.InitiativePage) error {
	if _, err := fmt.Fprintf(w, "%s\t%s\n", p.Address, p.State); err != nil {
		return err
	}
	for _, m := range p.Milestones {
		if _, err := fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", m.Number, m.Slug, m.State, m.Goal); err != nil {
			return err
		}
	}
	for _, u := range p.Unparsed {
		if _, err := fmt.Fprintf(w, "unparsed\t%s\t%s\n", u.Section, u.Raw); err != nil {
			return err
		}
	}
	return nil
}
