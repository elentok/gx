// Package initiatives is `gx initiatives`: the verbs that write Initiatives
// straight to the ticket store, with no server call.
package initiatives

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/epicname"
	"github.com/elentok/gx/tickets/initiative"
)

// Deps is the runtime context of every command in this package.
type Deps struct {
	cmdbase.Env
}

// New builds `gx initiatives`.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "initiatives", Short: "create and manage Initiatives"}
	cmd.AddCommand(newNew(d), newDeferred(d), newAssumption(d))
	return cmd
}

func newNew(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "new <project:slug> <idea>",
		Short: "create an Initiative holding only your idea (no worktree, no design step)",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.Cwd()
			if err != nil {
				return err
			}
			return runNew(d.Dirs(), cwd, args[0], args[1], c.OutOrStdout())
		},
	}
}

// runNew creates the Initiative epic named by arg and prints project:slug.
// Every check runs before the first write.
func runNew(cfgDirs config.Dirs, cwd, arg, idea string, w io.Writer) error {
	idea = strings.TrimSpace(idea)
	if idea == "" {
		return errors.New("the idea is empty")
	}
	projectDir, project, slug, err := tickets.ResolveEpicTarget(cfgDirs, cwd, arg)
	if err != nil {
		return err
	}
	for _, name := range []string{slug, slug + "-design"} {
		if err := epicname.Refuse(projectDir, project, name); err != nil {
			return err
		}
	}
	records, err := conversations.DirIn(cfgDirs)
	if err != nil {
		return err
	}
	body := "## " + initiative.InitialPrompt + "\n\n" + idea + "\n"
	fields := tickets.EpicFields{Kind: tickets.KindInitiative, Status: "open"}
	if _, err := tickets.CreateEpic(records, projectDir, slug, fields, body); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s:%s\n", project, slug)
	return err
}
