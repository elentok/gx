// Package gitops holds the git-work verbs of the gx CLI: push, merge, cleanup,
// bump, stashify, run and the `worktrees` subcommands list, abs-path and clone.
package gitops

import (
	"io"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/git"

	"github.com/spf13/cobra"
)

// Deps is the shared env plus the hooks these commands own. A nil
// ChoosePushDivergence means the real menu; ConfirmForce has no default
// because root also hands it to `gx doctor`.
type Deps struct {
	cmdbase.Env
	ChoosePushDivergence func(io.Reader, io.Writer, *git.PushDivergence) (int, error)
	ConfirmForce         func(string) (bool, error)
}

// DefaultDeps returns the production hooks of this package.
func DefaultDeps() Deps {
	return Deps{ChoosePushDivergence: choosePushDivergence}
}

// WorktreeCommands returns the `gx worktrees` subcommands that are git work;
// root adds them to the command that opens the worktree UI.
func WorktreeCommands(d Deps) []*cobra.Command {
	return []*cobra.Command{
		{
			Use:   "list",
			Short: "list worktree names",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runListWorktrees(d)
			},
		},
		{
			Use:   "abs-path <name>",
			Short: "print absolute path of a worktree",
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return runWorktreeAbsPath(args[0], d)
			},
		},
		{
			Use:   "clone <url> [dir]",
			Short: "clone using the .bare trick",
			RunE: func(_ *cobra.Command, args []string) error {
				return runCloneWT(args, d)
			},
		},
	}
}

// NewPush builds `gx push`.
func NewPush(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:     "push",
		Aliases: []string{"ps"},
		Short:   "push the current branch (with divergence handling)",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runPush(d)
		},
	}
}

// NewMerge builds `gx merge`.
func NewMerge(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "merge <branch>",
		Short: "fast-forward-only merge a branch/worktree onto trunk (deterministic core)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.Getwd()
			if err != nil {
				return err
			}
			return runMerge(d.Dirs(), cwd, args[0], jsonOut, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

// NewCleanup builds `gx cleanup`.
func NewCleanup(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "epic/worktree housekeeping",
	}
	var jsonOut bool
	scan := &cobra.Command{
		Use:   "scan",
		Short: "report epic done/merged/code-review status and repo housekeeping",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cwd, err := d.Getwd()
			if err != nil {
				return err
			}
			return runCleanupScan(d.Dirs(), cwd, jsonOut, c.OutOrStdout())
		},
	}
	scan.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(scan)
	return cmd
}

// NewBump builds `gx bump`.
func NewBump(d Deps) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "bump [major|minor|patch]",
		Short: "create a version tag and optionally push",
		RunE: func(_ *cobra.Command, args []string) error {
			return runBump(args, d, yes)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "push commits and tag to origin without prompting")
	return cmd
}

// NewStashify builds `gx stashify`.
func NewStashify(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "stashify <cmd...>",
		Short: "stash, run command, pop",
		// Pass flags through to the wrapped command untouched.
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runStashify(args, d)
		},
	}
}

// NewRun builds the hidden `gx run`.
func NewRun(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "run <cmd...>",
		Short: "run a command, keeping the pane open on failure",
		// Internal helper used by split/tab launches; hidden from --help.
		Hidden: true,
		// Pass everything after "run" to the wrapped command untouched.
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runRun(args, d)
		},
	}
}
