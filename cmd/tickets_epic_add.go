package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

// runTicketsEpicAdd creates the epic named by arg ("project:epic" or a bare
// epic in the cwd's project) and prints its address-less name, project:epic.
// An unknown project refuses with tickets.CodeUnknownProject before anything is
// written.
func runTicketsEpicAdd(cwd, arg, kind, status, body string, w io.Writer) error {
	projectDir, project, name, err := resolveEpicTarget(cwd, arg)
	if err != nil {
		return err
	}
	if _, err := tickets.CreateEpic(projectDir, name, tickets.EpicFields{Kind: kind, Status: status}, body); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s:%s\n", project, name)
	return err
}

// resolveEpicTarget splits arg into its project directory, project name and
// epic name. A bare epic name belongs to the cwd's project.
func resolveEpicTarget(cwd, arg string) (projectDir, project, name string, err error) {
	project, name, hasProject := strings.Cut(arg, ":")
	if !hasProject {
		name = arg
		if projectDir, err = ticketRoot(cwd); err != nil {
			return "", "", "", err
		}
		return projectDir, tickets.ProjectName(projectDir), name, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", "", "", err
	}
	dirs, err := tickets.ProjectDirs(cfg.TicketStore.Path)
	if err != nil {
		return "", "", "", err
	}
	for _, dir := range dirs {
		if tickets.ProjectName(dir) == project {
			return dir, project, name, nil
		}
	}
	return "", "", "", &tickets.AddressError{Code: tickets.CodeUnknownProject, Msg: fmt.Sprintf("no project %q", project)}
}

func newTicketsEpicCmd(d deps) *cobra.Command {
	epicCmd := &cobra.Command{
		Use:   "epic",
		Short: "create and manage epics",
	}
	var kind, status, body string
	addCmd := &cobra.Command{
		Use:   "add <project:epic>",
		Short: "create an epic's ticket.md (kind map or loop, status draft or open)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			if body != "" {
				if body, err = readBodyArg(d, body); err != nil {
					return err
				}
			}
			return runTicketsEpicAdd(cwd, args[0], kind, status, body, c.OutOrStdout())
		},
	}
	addCmd.Flags().StringVar(&kind, "kind", "", "map or loop (required unless the body's frontmatter sets it)")
	addCmd.Flags().StringVar(&status, "status", "", "draft or open (default draft)")
	addCmd.Flags().StringVar(&body, "body", "", "epic body, or - to read it from stdin; its frontmatter may set kind, status, destination_unconfirmed")
	epicCmd.AddCommand(addCmd, newTicketsEpicSetCmd(d))
	return epicCmd
}

// newTicketsEpicSetCmd is `tickets set project:epic` under the epic group;
// --blocked-by takes whole-epic tokens ("other" or "project:other") and runs
// the same project-wide check as a ticket's, writing nothing on failure.
func newTicketsEpicSetCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <project:epic>",
		Short: "validated writes to an epic's ticket.md (kind, status, destination_unconfirmed, blocked_by)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			projectDir, project, name, err := resolveEpicTarget(cwd, args[0])
			if err != nil {
				return err
			}
			path, err := tickets.EpicTicketMDPath(projectDir, project, name)
			if err != nil {
				return err
			}
			return runEpicSet(c, path, c.OutOrStdout(), d.getwd)
		},
	}
	cmd.Flags().String("kind", "", "set kind (map or loop)")
	cmd.Flags().String("status", "", "set status (draft or open)")
	addDestinationUnconfirmedFlags(cmd, "set destination_unconfirmed (true/false)")
	cmd.Flags().String("blocked-by", "", "set blocked_by (comma-separated whole-epic tokens, e.g. other or project:other)")
	cmd.Flags().Bool("json", false, "emit {address, path, updated, via, actor} JSON instead of text")
	return cmd
}
