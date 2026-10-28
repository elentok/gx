package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
)

func newCheckCmd(d deps) *cobra.Command {
	var staged, full bool
	var since, kinds string
	cmd := &cobra.Command{
		Use:   "check [--staged | --since <ref> | --full] [--kind <list>]",
		Short: "run the project's format, lint, typecheck and test checks",
		Long: `Run the commands in .gx/checks.json (or the checks key of the project's
project.json) on the files changed against HEAD plus untracked files, and print
one line per kind plus the path of the summary and logs.

--staged selects the staged files, --since <ref> the files changed in
<ref>...HEAD, --full runs each kind's full command on every tracked file. They
exclude each other. --kind format,lint runs only the named kinds.

Exit 0 pass, 1 fail, 2 gx could not run, 3 no applicable checks.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runCheck(c, d, staged, full, since, kinds)
		},
	}
	cmd.Flags().BoolVar(&staged, "staged", false, "check the staged files")
	cmd.Flags().StringVar(&since, "since", "", "check the files changed in <ref>...HEAD")
	cmd.Flags().BoolVar(&full, "full", false, "run the full command on every tracked file")
	cmd.Flags().StringVar(&kinds, "kind", "", "comma-separated kinds to run (default: all)")
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return checkUsageError(c, err.Error())
	})
	return cmd
}

// checkUsageError prints one line and exits 2, so exit 1 keeps meaning "a
// check failed".
func checkUsageError(c *cobra.Command, msg string) error {
	fmt.Fprintf(c.OutOrStdout(), "check: %s\n", msg)
	return &ExitError{Code: check.ExitCantRun}
}

func runCheck(c *cobra.Command, d deps, staged, full bool, since, kinds string) error {
	opts := check.Options{}
	modes := 0
	if staged {
		opts.Mode = check.ModeStaged
		modes++
	}
	if since != "" {
		opts.Mode, opts.Since = check.ModeSince, since
		modes++
	}
	if full {
		opts.Mode = check.ModeFull
		modes++
	}
	if modes > 1 {
		return checkUsageError(c, "--staged, --since and --full exclude each other")
	}
	if kinds != "" {
		opts.Kinds = strings.Split(kinds, ",")
	}
	dir, err := d.getwd()
	if err != nil {
		return err
	}
	opts.Dir = dir
	opts.Env = d.env
	if opts.DataDir, err = d.dirs().DataDir(); err != nil {
		return err
	}
	opts.ProjectChecks = func() (json.RawMessage, error) { return projectChecks(d.dirs(), dir) }
	if cfg, err := d.dirs().Load(); err == nil {
		opts.TicketStore = cfg.TicketStore.Path
	}
	opts.ProjectFor = func(dir string) string { return check.ProjectForDirs(d.dirs(), dir) }

	res, err := check.Run(opts)
	if err != nil {
		return checkUsageError(c, err.Error())
	}
	fmt.Fprint(c.OutOrStdout(), res.Render())
	if res.Exit != check.ExitPass {
		return &ExitError{Code: res.Exit}
	}
	return nil
}

// projectChecks is the checks key of the ticket store's project.json for the
// repo at dir. An unregistered repo has none.
func projectChecks(dirs config.Dirs, dir string) (json.RawMessage, error) {
	projectDir, err := tickets.RootForDirs(dirs, dir)
	if errors.Is(err, tickets.ErrNoProject) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil {
		return nil, err
	}
	return pf.Checks, nil
}
