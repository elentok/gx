package ticketsflow

import (
	"errors"
	"fmt"
	"io"

	"github.com/elentok/gx/cmd/cmdbase"
	ticketscmd "github.com/elentok/gx/cmd/tickets"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/epicname"
	"github.com/spf13/cobra"
)

// runTicketsEpicAdd creates the epic named by arg ("project:epic" or a bare
// epic in the cwd's project) and prints its address-less name, project:epic.
// An unknown project refuses with tickets.CodeUnknownProject before anything is
// written.
func runTicketsEpicAdd(cfgDirs config.Dirs, cwd, arg, kind, status, body string, w io.Writer) error {
	return runTicketsEpicAddFields(cfgDirs, cwd, arg, tickets.EpicFields{Kind: kind, Status: status}, body, w)
}

// runTicketsEpicAddFields is runTicketsEpicAdd with the full set of flag fields.
func runTicketsEpicAddFields(cfgDirs config.Dirs, cwd, arg string, fields tickets.EpicFields, body string, w io.Writer) error {
	projectDir, project, name, err := resolveEpicTarget(cfgDirs, cwd, arg)
	if err != nil {
		return err
	}
	if err := epicname.Refuse(projectDir, project, name); err != nil {
		return err
	}
	if fields.Initiative != "" && fields.Milestone != "" {
		for _, sibling := range []string{name + "-design", name + "-impl"} {
			why, err := epicname.Taken(projectDir, project, sibling)
			if err != nil {
				return err
			}
			if why != "" {
				return errors.New(why)
			}
		}
	}
	records, err := conversations.DirIn(cfgDirs)
	if err != nil {
		return err
	}
	if _, err := tickets.CreateEpic(records, projectDir, name, fields, body); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s:%s\n", project, name)
	return err
}

var resolveEpicTarget = tickets.ResolveEpicTarget

func newTicketsEpicCmd(d Deps) *cobra.Command {
	epicCmd := &cobra.Command{
		Use:   "epic",
		Short: "create and manage epics",
	}
	var kind, status, body, initiative, milestone string
	addCmd := &cobra.Command{
		Use:   "add <project:epic>",
		Short: "create an epic's ticket.md (kind map, loop, backlog or initiative, status draft or open)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.Getwd()
			if err != nil {
				return err
			}
			if body != "" {
				if body, err = cmdbase.ReadBodyArg(d.Stdin, body); err != nil {
					return err
				}
			}
			fields := tickets.EpicFields{Kind: kind, Status: status, Initiative: initiative, Milestone: milestone}
			return runTicketsEpicAddFields(d.dirs(), cwd, args[0], fields, body, c.OutOrStdout())
		},
	}
	addCmd.Flags().StringVar(&kind, "kind", "", "map, loop, backlog or initiative (required unless the body's frontmatter sets it)")
	addCmd.Flags().StringVar(&initiative, "initiative", "", "map only: the initiative the map belongs to")
	addCmd.Flags().StringVar(&milestone, "milestone", "", "map only: the milestone the map belongs to")
	addCmd.Flags().StringVar(&status, "status", "", "draft or open (default draft)")
	addCmd.Flags().StringVar(&body, "body", "", "epic body, or - to read it from stdin; its frontmatter may set kind, status, destination_unconfirmed")
	nextNameCmd := &cobra.Command{
		Use:   "next-name <project:base>",
		Short: "print project:base, or project:base-2, -3 … when base is taken (exists, or used by an earlier epic)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.Getwd()
			if err != nil {
				return err
			}
			projectDir, project, base, err := resolveEpicTarget(d.dirs(), cwd, args[0])
			if err != nil {
				return err
			}
			name, err := epicname.Next(projectDir, project, base)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "%s:%s\n", project, name)
			return err
		},
	}
	epicCmd.AddCommand(addCmd, nextNameCmd, newTicketsEpicSetCmd(d))
	return epicCmd
}

// newTicketsEpicSetCmd is `tickets set project:epic` under the epic group;
// --blocked-by takes whole-epic tokens ("other" or "project:other") and runs
// the same project-wide check as a ticket's, writing nothing on failure.
func newTicketsEpicSetCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <project:epic>",
		Short: "validated writes to an epic's ticket.md (kind, status, destination_unconfirmed, blocked_by)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.Getwd()
			if err != nil {
				return err
			}
			projectDir, project, name, err := resolveEpicTarget(d.dirs(), cwd, args[0])
			if err != nil {
				return err
			}
			path, err := tickets.EpicTicketMDPath(projectDir, project, name)
			if err != nil {
				return err
			}
			return ticketscmd.RunEpicSet(d.dirs(), c, path, c.OutOrStdout(), d.Getwd, d.Dial())
		},
	}
	cmd.Flags().String("kind", "", "set kind (map, loop, backlog or initiative; never to or from backlog or initiative)")
	cmd.Flags().String("status", "", "set status (draft or open)")
	cmd.Flags().String("destination-unconfirmed", "", "set destination_unconfirmed (true/false)")
	cmd.Flags().String("blocked-by", "", "set blocked_by (comma-separated whole-epic tokens, e.g. other or project:other)")
	cmd.Flags().Bool("json", false, "emit {address, path, updated, via, actor} JSON instead of text")
	return cmd
}
