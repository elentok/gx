package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// newServerTriageCmd groups the Triage verbs that talk to the server.
func newServerTriageCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "triage", Short: "Triage calls to the running server"}

	var jsonOut bool
	changed := &cobra.Command{
		Use:   "changed <project>",
		Short: "tell the server a project's Triage record changed so the web updates at once",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			if err := cl.Triage.Changed(ctx, args[0]); err != nil {
				return fmt.Errorf("server ping failed: %w", err)
			}
			if jsonOut {
				return cmdbase.WriteStamped(c.OutOrStdout(), wire.TriageChangedResult{Project: args[0]}, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
			}
			return nil
		},
	}
	changed.Flags().BoolVar(&jsonOut, "json", false, "emit the acknowledged project as JSON")
	cmd.AddCommand(changed)

	var openJSON, openNew bool
	open := &cobra.Command{
		Use:   "open <project>",
		Short: "start a project's Triage, or print the session of the live one",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Triage.Open(ctx, wire.TriageOpenRequest{Project: args[0], New: openNew})
			if err != nil {
				return fmt.Errorf("server write failed: %w", err)
			}
			if openJSON {
				return cmdbase.WriteStamped(c.OutOrStdout(), res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
			}
			if res.Refused {
				return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
			}
			if res.Session != nil {
				_, err = fmt.Fprintln(c.OutOrStdout(), res.Session.ID)
			}
			return err
		},
	}
	open.Flags().BoolVar(&openJSON, "json", false, "emit the structured result (or refusal) as JSON")
	open.Flags().BoolVar(&openNew, "new", false, "stop the live session and start a fresh one")
	cmd.AddCommand(open)

	editOut := func(c *cobra.Command, res wire.TriageEditResult, asJSON bool) error {
		if asJSON {
			return cmdbase.WriteStamped(c.OutOrStdout(), res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
		}
		if res.Refused {
			return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
		}
		_, err := fmt.Fprintln(c.OutOrStdout(), res.Revision)
		return err
	}

	var rowJSON, rowKeep bool
	var rowAction, rowDest string
	var rowAnswers []string
	var rowRevision int
	row := &cobra.Command{
		Use:   "row <project> <address>",
		Short: "save a row's action, destination, answers or Keep for later on the record revision read",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			answers := map[string]string{}
			for _, a := range rowAnswers {
				id, text, ok := strings.Cut(a, "=")
				if !ok {
					return fmt.Errorf("--answer %q is not id=text", a)
				}
				answers[id] = text
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Triage.SaveRow(ctx, wire.TriageRowRequest{
				Project: args[0], Revision: rowRevision, Address: args[1], Action: rowAction,
				Destination: rowDest, Answers: answers, KeepForLater: rowKeep,
			})
			if err != nil {
				return fmt.Errorf("server write failed: %w", err)
			}
			return editOut(c, res, rowJSON)
		},
	}
	row.Flags().BoolVar(&rowJSON, "json", false, "emit the structured result (or refusal) as JSON")
	row.Flags().IntVar(&rowRevision, "revision", 0, "the record revision read")
	row.Flags().StringVar(&rowAction, "action", "", "ready, map, merge, close or keep")
	row.Flags().StringVar(&rowDest, "destination", "", "epic, new map epic name, or the row address merged into")
	row.Flags().StringArrayVar(&rowAnswers, "answer", nil, "a question answer as id=text (repeatable)")
	row.Flags().BoolVar(&rowKeep, "keep-for-later", false, "keep the follow-up for later")
	cmd.AddCommand(row)

	var abandonJSON bool
	var abandonRevision int
	abandon := &cobra.Command{
		Use:   "abandon <project>",
		Short: "abandon a project's live Triage and move it to history",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Triage.Abandon(ctx, wire.TriageAbandonRequest{Project: args[0], Revision: abandonRevision})
			if err != nil {
				return fmt.Errorf("server write failed: %w", err)
			}
			return editOut(c, res, abandonJSON)
		},
	}
	abandon.Flags().BoolVar(&abandonJSON, "json", false, "emit the structured result (or refusal) as JSON")
	abandon.Flags().IntVar(&abandonRevision, "revision", 0, "the record revision read")
	cmd.AddCommand(abandon)

	var showJSON bool
	show := &cobra.Command{
		Use:   "show <project>",
		Short: "show a project's Triage as the web page reads it",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), showJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.TriageView, error) {
					return cl.Triage.Show(ctx, args[0])
				},
				func(w io.Writer, v wire.TriageView) error {
					_, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d rows\t%s\n", v.Project, v.State, v.StepNote, len(v.Rows), v.ID)
					return err
				})
		},
	}
	show.Flags().BoolVar(&showJSON, "json", false, "emit the whole view as JSON")
	cmd.AddCommand(show)
	return cmd
}
