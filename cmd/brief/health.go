package brief

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/check"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/suitehealth"
)

// usageFlagErrors makes a flag or argument error exit 2 with one line, like
// every other brief usage error.
func usageFlagErrors(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return briefUsageError(c, err.Error())
	})
}

func newHealth(d Deps) *cobra.Command {
	var limit int
	var allRuns bool
	var cDir string
	cmd := &cobra.Command{
		Use:   "health [-C dir] [--limit N] [--all-runs]",
		Short: "first-attempt pass rate and slow or flaky tests of gx check runs",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return briefUsageError(c, fmt.Sprintf("unknown health argument %q", args[0]))
			}
			if c.Flags().Changed("limit") && limit <= 0 {
				return briefUsageError(c, fmt.Sprintf("bad --limit %d", limit))
			}
			dir, err := resolveDir(d, cDir)
			if err != nil {
				return err
			}
			dataDir, err := d.Dirs().DataDir()
			if err != nil {
				return err
			}
			opts := suitehealth.Options{Limit: limit, AllRuns: allRuns, DataDir: dataDir, Project: check.ProjectForDirs(d.Dirs(), dir)}
			if code := suitehealth.Health(opts, c.OutOrStdout()); code != suitehealth.ExitPass {
				return &cmdbase.ExitError{Code: code}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "count only the newest N runs")
	cmd.Flags().BoolVar(&allRuns, "all-runs", false, "count every run, not only full trunk runs")
	cmd.Flags().StringVarP(&cDir, "dir", "C", "", "project directory (default: the current one)")
	usageFlagErrors(cmd)
	return cmd
}

func newFailures(d Deps) *cobra.Command {
	var byFile, runs bool
	cmd := &cobra.Command{
		Use:   "failures [--by-file] [log [test]] | failures --runs [N]",
		Short: "print the failing tests of a brief log with their full text",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			dataDir, err := d.Dirs().DataDir()
			if err != nil {
				return err
			}
			if runs {
				n := 10
				if len(args) > 1 {
					return briefUsageError(c, "--runs takes at most one number")
				}
				if len(args) == 1 {
					if n, err = strconv.Atoi(args[0]); err != nil || n <= 0 {
						return briefUsageError(c, fmt.Sprintf("bad --runs count %q", args[0]))
					}
				}
				if code := brief.FailureRuns(dataDir, n, c.OutOrStdout()); code != brief.ExitPass {
					return &cmdbase.ExitError{Code: code}
				}
				return nil
			}
			var logPath, test string
			if len(args) > 0 {
				logPath = args[0]
			}
			if len(args) > 1 {
				test = args[1]
			}
			if code := brief.Failures(dataDir, logPath, test, byFile, c.OutOrStdout()); code != brief.ExitPass {
				return &cmdbase.ExitError{Code: code}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&byFile, "by-file", false, "print one count per file instead of the failures")
	cmd.Flags().BoolVar(&runs, "runs", false, "list the failing e2e files of the last N playwright runs (default 10)")
	usageFlagErrors(cmd)
	return cmd
}

// subcommand finds a registered subcommand of c by name.
func subcommand(c *cobra.Command, name string) *cobra.Command {
	for _, sub := range c.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

// runSub runs a subcommand reached through `gx brief -C dir <sub>`: cobra
// does not dispatch there, because the parent reads -C by hand.
func runSub(sub *cobra.Command, cFlag string, args []string) error {
	if cFlag != "" && sub.Flags().Lookup("dir") != nil {
		args = append([]string{"-C", cFlag}, args...)
	}
	if err := sub.ParseFlags(args); err != nil {
		return briefUsageError(sub, err.Error())
	}
	return sub.RunE(sub, sub.Flags().Args())
}
