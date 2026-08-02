package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"

	"github.com/spf13/cobra"
)

func newWorktreesCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "worktrees",
		Aliases: []string{"wt"},
		Short:   "open the worktree UI",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.runWorktrees("")
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "list worktree names",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runListWorktrees(d)
			},
		},
		&cobra.Command{
			Use:   "abs-path <name>",
			Short: "print absolute path of a worktree",
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return runWorktreeAbsPath(args[0], d)
			},
		},
		&cobra.Command{
			Use:   "clone <url> [dir]",
			Short: "clone using the .bare trick",
			RunE: func(_ *cobra.Command, args []string) error {
				return runCloneWT(args, d)
			},
		},
	)
	return cmd
}

func newPushCmd(d deps) *cobra.Command {
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

func newStatusCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:     "status [path]",
		Aliases: []string{"s"},
		Short:   "open the status UI",
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("usage: gx status|s [path]")
			}
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			return d.runStatus(target)
		},
	}
}

func newLogCmd(d deps) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "log [hash-or-ref]",
		Short: "open the log UI",
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("usage: gx log [hash-or-ref]")
			}
			ref := ""
			if len(args) == 1 {
				ref = args[0]
			}
			resolved, err := resolveLogFile(d, file)
			if err != nil {
				return err
			}
			return d.runLog(LogOptions{Ref: ref, File: resolved})
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "open the log pre-filtered to a file (follows renames)")
	_ = cmd.MarkFlagFilename("file")
	return cmd
}

// resolveLogFile turns a user-supplied -f path (cwd-relative or absolute) into
// the repo-relative form the log filter expects. Empty input stays empty so a
// plain `gx log` needs no repo lookup. Paths outside the worktree are rejected.
func resolveLogFile(d deps, file string) (string, error) {
	if strings.TrimSpace(file) == "" {
		return "", nil
	}
	cwd, err := d.getwd()
	if err != nil {
		return "", err
	}
	root, err := git.WorktreeRoot(cwd)
	if err != nil {
		return "", err
	}
	// git reports the canonical (symlink-resolved) root; align cwd so the two
	// share a prefix before computing the repo-relative path.
	if canon, e := filepath.EvalSymlinks(cwd); e == nil {
		cwd = canon
	}
	return git.RepoRelativePath(cwd, file, root)
}

func newShowCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show [hash-or-ref]",
		Short: "open the commit UI",
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("usage: gx show [hash-or-ref]")
			}
			ref := ""
			if len(args) == 1 {
				ref = args[0]
			}
			return d.runShow(ref)
		},
	}
}

func newStashCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "stash",
		Short: "open the stash UI",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.runStash()
		},
	}
}

func newPRsCmd(d deps) *cobra.Command {
	var allRepos bool
	cmd := &cobra.Command{
		Use:   "prs",
		Short: "open the PRs UI",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.runPRs(allRepos)
		},
	}
	cmd.Flags().BoolVar(&allRepos, "all", false, "show outgoing PRs across all repos, not just the current one")
	return cmd
}

func newTicketsCmd(d deps) *cobra.Command {
	var allRepos bool
	cmd := &cobra.Command{
		Use:     "tickets",
		Aliases: []string{"tk"},
		Short:   "open the tickets UI",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.runTickets(allRepos)
		},
	}
	cmd.Flags().BoolVar(&allRepos, "all", false, "show every worktree's .scratch/ tickets, not just the current one")
	cmd.AddCommand(&cobra.Command{
		Use:   "validate <path>",
		Short: "validate a ticket file's frontmatter",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runTicketsValidate(args[0], c.OutOrStdout())
		},
	})
	return cmd
}

func newConfigCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "manage gx configuration",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "edit",
			Short: "open the config file in $EDITOR (creates it if missing)",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runEditConfig(d)
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "print the effective (merged) config as JSON",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runConfigShow(d)
			},
		},
		&cobra.Command{
			Use:   "defaults",
			Short: "print the built-in default config as JSON",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runConfigDefaults(d)
			},
		},
	)
	return cmd
}

func newBumpCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "bump [major|minor|patch]",
		Short: "create a version tag and optionally push",
		RunE: func(_ *cobra.Command, args []string) error {
			return runBump(args, d)
		},
	}
}

func newStashifyCmd(d deps) *cobra.Command {
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

func newRunCmd(d deps) *cobra.Command {
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

func newDoctorCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor [--fix] [--pause]",
		Short: "check the repo for common configuration issues",
		// runDoctor does its own flag parsing.
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runDoctor(args, d)
		},
	}
}

func newVersionCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the gx version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runVersion(d.stdout)
		},
	}
}
