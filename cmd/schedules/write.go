package schedules

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/wire/ops"
)

var (
	weekdays = []string{"mon", "tue", "wed", "thu", "fri"}
	weekends = []string{"sat", "sun"}
	everyDay = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
)

// expandDays turns --day values (repeated or comma lists, day names or the
// words weekdays, weekends and every-day) into three-letter day names. A word
// it does not know passes through for the server to refuse.
func expandDays(vals []string) []string {
	var out []string
	for _, v := range vals {
		for _, w := range strings.Split(v, ",") {
			w = strings.ToLower(strings.TrimSpace(w))
			switch w {
			case "":
			case "weekdays":
				out = append(out, weekdays...)
			case "weekends":
				out = append(out, weekends...)
			case "every-day", "everyday":
				out = append(out, everyDay...)
			default:
				if len(w) > 3 && strings.HasSuffix(w, "day") {
					w = w[:3]
				}
				out = append(out, w)
			}
		}
	}
	return out
}

// writeFlags are the flags create and edit share.
type writeFlags struct {
	name, prompt, project, harness, tier string
	days, times                          []string
	lateLimit                            float64
	jsonOut                              bool
}

func (f *writeFlags) bind(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.name, "name", "", "display name")
	fl.StringVar(&f.prompt, "prompt", "", "the prompt to run, or - for stdin")
	fl.StringSliceVar(&f.days, "day", nil, "run day: mon..sun, weekdays, weekends or every-day (repeatable or comma list)")
	fl.StringSliceVar(&f.times, "time", nil, "run time HH:MM, local (repeatable or comma list)")
	fl.StringVar(&f.project, "project", "", "project name")
	fl.StringVar(&f.harness, "harness", "", "claude or codex; empty follows the Default")
	fl.StringVar(&f.tier, "tier", "", "light, worker or strongest; empty runs on worker")
	fl.Float64Var(&f.lateLimit, "late-limit", 0, "Late limit in hours")
	fl.BoolVar(&f.jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
}

func readPrompt(prompt string, stdin io.Reader) (string, error) {
	if prompt != "-" {
		return prompt, nil
	}
	b, err := io.ReadAll(io.LimitReader(stdin, wire.MaxSessionPromptBytes+1))
	if err != nil {
		return "", fmt.Errorf("read prompt from stdin: %w", err)
	}
	return string(b), nil
}

func newCreate(d Deps) *cobra.Command {
	var f writeFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "create a Schedule, switched on on this machine",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			prompt, err := readPrompt(f.prompt, c.InOrStdin())
			if err != nil {
				return err
			}
			req := ops.ScheduleCreateRequest{
				Name: f.name, Prompt: prompt, Days: expandDays(f.days), Times: f.times, Project: f.project, Harness: f.harness, Tier: f.tier,
			}
			if c.Flags().Changed("late-limit") {
				req.LateLimitHours = &f.lateLimit
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Schedules.Create(ctx(c), req)
			return finish(c.OutOrStdout(), f.jsonOut, res, res.Refused, res.Reason, res.Message, err, printAddress)
		},
	}
	f.bind(cmd)
	return cmd
}

func newEdit(d Deps) *cobra.Command {
	var f writeFlags
	cmd := &cobra.Command{
		Use:   "edit <project:slug>",
		Short: "change the given fields of a Schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			project, slug, ok := strings.Cut(args[0], ":")
			if !ok || project == "" || slug == "" {
				return fmt.Errorf("want <project:slug>, got %q", args[0])
			}
			req := ops.ScheduleEditRequest{Project: project, Slug: slug}
			changed := c.Flags().Changed
			if changed("project") {
				req.NewProject = &f.project
			}
			if changed("name") {
				req.Name = &f.name
			}
			if changed("prompt") {
				prompt, err := readPrompt(f.prompt, c.InOrStdin())
				if err != nil {
					return err
				}
				req.Prompt = &prompt
			}
			if changed("day") {
				req.Days = expandDays(f.days)
			}
			if changed("time") {
				req.Times = f.times
			}
			if changed("harness") {
				req.Harness = &f.harness
			}
			if changed("tier") {
				req.Tier = &f.tier
			}
			if changed("late-limit") {
				req.LateLimitHours = &f.lateLimit
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Schedules.Edit(ctx(c), req)
			return finish(c.OutOrStdout(), f.jsonOut, res, res.Refused, res.Reason, res.Message, err, printAddress)
		},
	}
	f.bind(cmd)
	return cmd
}

// parseAddress splits <project:slug>.
func parseAddress(arg string) (project, slug string, err error) {
	project, slug, ok := strings.Cut(arg, ":")
	if !ok || project == "" || slug == "" {
		return "", "", fmt.Errorf("want <project:slug>, got %q", arg)
	}
	return project, slug, nil
}

func newSwitch(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:       "switch <project:slug> on|off",
		Short:     "turn a Schedule on or off on this machine",
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{"on", "off"},
		RunE: func(c *cobra.Command, args []string) error {
			project, slug, err := parseAddress(args[0])
			if err != nil {
				return err
			}
			if args[1] != "on" && args[1] != "off" {
				return fmt.Errorf("want on or off, got %q", args[1])
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Schedules.Switch(ctx(c), ops.ScheduleSwitchRequest{Project: project, Slug: slug, On: args[1] == "on"})
			return finish(c.OutOrStdout(), jsonOut, res, res.Refused, res.Reason, res.Message, err, printAddress)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func newDelete(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "delete <project:slug>",
		Short: "delete a Schedule; its queued runs that never started are cancelled",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			project, slug, err := parseAddress(args[0])
			if err != nil {
				return err
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Schedules.Delete(ctx(c), ops.ScheduleDeleteRequest{Project: project, Slug: slug})
			return finish(c.OutOrStdout(), jsonOut, res, res.Refused, res.Reason, res.Message, err, func(w io.Writer, _ ops.ScheduleDeleteResult) error {
				_, err := fmt.Fprintf(w, "%s:%s\n", project, slug)
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func newRunNow(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "run-now <project:slug>",
		Short: "start one run of a Schedule now, whatever its switch says",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			project, slug, err := parseAddress(args[0])
			if err != nil {
				return err
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Schedules.RunNow(ctx(c), ops.ScheduleRunNowRequest{Project: project, Slug: slug})
			message := res.Message
			if res.AttentionAddress != "" {
				message += " (needs attention: " + res.AttentionAddress + ")"
			}
			return finish(c.OutOrStdout(), jsonOut, res, res.Refused, res.Reason, message, err, func(w io.Writer, res ops.ScheduleRunNowResult) error {
				_, err := fmt.Fprintln(w, res.Address)
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func printAddress(w io.Writer, res ops.ScheduleShowResult) error {
	_, err := fmt.Fprintf(w, "%s:%s\n", res.Schedule.Project, res.Schedule.Slug)
	return err
}
