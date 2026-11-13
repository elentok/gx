// Package schedules is `gx schedules`: the verbs that read stored Schedules
// through the API.
package schedules

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// Deps is the runtime context of every command in this package.
type Deps struct {
	cmdbase.Env
}

// New builds `gx schedules`.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "schedules", Short: "list, show, create, edit, switch and delete Schedules (asks the server)"}
	cmd.AddCommand(newList(d), newShow(d), newCreate(d), newEdit(d), newSwitch(d), newDelete(d), newRunNow(d))
	return cmd
}

func newList(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list every Schedule with its days, times and next run",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Schedules.List(ctx(c))
			return finish(c.OutOrStdout(), jsonOut, res, res.Refused, res.Reason, res.Message, err, printList)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func newShow(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <project:slug>",
		Short: "show one Schedule with its prompt and last runs",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			project, slug, ok := strings.Cut(args[0], ":")
			if !ok || project == "" || slug == "" {
				return fmt.Errorf("want <project:slug>, got %q", args[0])
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Schedules.Show(ctx(c), project, slug)
			return finish(c.OutOrStdout(), jsonOut, res, res.Refused, res.Reason, res.Message, err, printShow)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func ctx(c *cobra.Command) context.Context {
	if c.Context() != nil {
		return c.Context()
	}
	return context.Background()
}

// finish prints res, or the refusal. A server that is not running is a refusal.
func finish[R any](w io.Writer, jsonOut bool, res R, refused bool, reason, message string, err error, print func(io.Writer, R) error) error {
	switch {
	case apiclient.IsNotRunning(err):
		return fmt.Errorf("refused (%s): %s", wire.ReasonServerNotRunning, "no server is running; start it with `gx server start`")
	case err != nil:
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenanceRefusal(w, res, refused)
	}
	if refused {
		return fmt.Errorf("refused (%s): %s", reason, message)
	}
	return print(w, res)
}

func printList(w io.Writer, res ops.SchedulesResult) error {
	for _, s := range res.Schedules {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, strings.Join(s.Days, ","), strings.Join(s.Times, ","), nextText(s)); err != nil {
			return err
		}
	}
	return nil
}

func printShow(w io.Writer, res ops.ScheduleShowResult) error {
	s := res.Schedule
	harness := s.Harness
	if harness == "" {
		harness = "default (" + s.DefaultHarness + ")"
	}
	tier := s.Tier
	if tier == "" {
		tier = "worker"
	}
	if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\nproject: %s\nharness: %s\ntier: %s\nlate limit: %gh\nprompt:\n%s\n", s.Name, strings.Join(s.Days, ","), strings.Join(s.Times, ","), nextText(s), s.Project, harness, tier, s.LateLimitHours, res.Prompt); err != nil {
		return err
	}
	for _, r := range res.LastRuns {
		line := r.Result
		if r.Reason != "" {
			line = r.ReasonText
		}
		if _, err := fmt.Fprintf(w, "run %s\t%s\t%s\t%s\n", r.ScheduledFor, r.Status, r.Address, line); err != nil {
			return err
		}
	}
	return nil
}

func nextText(s ops.ScheduleInfo) string {
	switch {
	case !s.ProjectFound:
		return "project not found"
	case !s.Switch:
		return "off here"
	case s.NextRun == "":
		return "no next run"
	}
	t, err := time.Parse(time.RFC3339, s.NextRun)
	if err != nil {
		return s.NextRun
	}
	return t.Local().Format("Mon 2006-01-02 15:04")
}
