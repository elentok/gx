package ticketsflow

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	ticketscmd "github.com/elentok/gx/cmd/tickets"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/epicname"
	"github.com/elentok/gx/tickets/ticketmove"
	"github.com/spf13/cobra"
)

// runTicketsStartDesign creates the map from the backlog's tickets (or carries on
// an unfinished run) and prints the map's address. active is the queued-or-live
// check applied to each ticket.
func runTicketsStartDesign(cfgDirs config.Dirs, cwd, backlogArg, mapName, title string, ids []string, active func(tickets.Address) error, w io.Writer) error {
	projectDir, project, backlog, err := resolveEpicTarget(cfgDirs, cwd, backlogArg)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(projectDir, mapName)); err != nil {
		// A new map: a rerun keeps its own, already taken, name.
		if err := epicname.Refuse(projectDir, project, mapName); err != nil {
			return err
		}
	}
	records, err := conversations.DirIn(cfgDirs)
	if err != nil {
		return err
	}
	if title == "" {
		title = mapName
	}
	name, err := ticketmove.StartDesign(ticketmove.StartDesignOpts{
		ConversationsDir: records, ProjectDir: projectDir, Project: project,
		Backlog: backlog, Map: mapName, Title: title, IDs: ids, Active: active,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s:%s\n", project, name)
	return err
}

func newTicketsStartDesignCmd(d Deps) *cobra.Command {
	var mapName, title string
	var ids []string
	cmd := &cobra.Command{
		Use:   "start-design <project:backlog>",
		Short: "create a charting map from a backlog's tickets and move them in; rerun it after a failure",
		Long: "Create a charting map (a Design) from tickets of a backlog and move them in.\n" +
			"Every ticket is checked first, so a refusal writes nothing. After a failure,\n" +
			"rerun with the same --map (with or without --ticket) to move the rest.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.Getwd()
			if err != nil {
				return err
			}
			dirs := d.dirs()
			active := func(a tickets.Address) error { return ticketscmd.RefuseIfActive(dirs, d.Dial(), a) }
			return runTicketsStartDesign(dirs, cwd, args[0], mapName, title, ids, active, c.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&mapName, "map", "", "name of the new map (check it with `tickets epic next-name`)")
	cmd.Flags().StringVar(&title, "title", "", "the approved title shown in the map (default: the map name)")
	cmd.Flags().StringSliceVar(&ids, "ticket", nil, "ids of the backlog's tickets to move, comma-separated; empty on a rerun reads them from the map's Notes")
	_ = cmd.MarkFlagRequired("map")
	return cmd
}
