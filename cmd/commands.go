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
	cmd := &cobra.Command{
		Use:     "tickets",
		Aliases: []string{"tk"},
		Short:   "open the tickets UI",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.runTickets()
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "validate <path>",
		Short: "validate a ticket file's frontmatter",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runTicketsValidate(args[0], c.OutOrStdout())
		},
	})
	cmd.AddCommand(newTicketsSetCmd(d))
	cmd.AddCommand(&cobra.Command{
		Use:   "migrate <path>",
		Short: "rewrite every ticket under a tracker root into the post-refactor frontmatter shape",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runTicketsMigrate(args[0], c.OutOrStdout())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "root",
		Short: "print the canonical .scratch root for the current repo",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsRoot(cwd, c.OutOrStdout())
		},
	})
	var mapsOnly bool
	epicsCmd := &cobra.Command{
		Use:   "epics",
		Short: "print bare epic slugs under the current repo's .scratch root",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsEpics(cwd, c.OutOrStdout(), mapsOnly)
		},
	}
	epicsCmd.Flags().BoolVar(&mapsOnly, "maps", false, "only print epics with a wayfinder map.md")
	cmd.AddCommand(epicsCmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "ensure-code-review <epic>",
		Short: "no-op if the epic has a code-review ticket, else stamp out a stub",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsEnsureCodeReview(resolveEpicArg(args[0], cwd), c.OutOrStdout())
		},
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			cwd, err := d.getwd()
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			names, err := completeEpicNames(cwd)
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "schema",
		Short: "print the ticket frontmatter schema (settable and read-only fields)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(c.OutOrStdout(), ticketsSchemaText)
			return err
		},
	})
	var addParent, addSlug string
	addCmd := &cobra.Command{
		Use:   "add <epic>",
		Short: "atomically allocate the next ticket ID and stamp out a stub file",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsAdd(resolveEpicArg(args[0], cwd), addParent, addSlug, c.OutOrStdout())
		},
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			cwd, err := d.getwd()
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			names, err := completeEpicNames(cwd)
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		},
	}
	addCmd.Flags().StringVar(&addParent, "parent", "", "allocate a lettered child of this ticket ID (or, if parent is itself lettered, one numeric level past it)")
	addCmd.Flags().StringVar(&addSlug, "slug", "", "descriptive filename slug, e.g. \"wire-tree-model-selection\" (required; stub lands at <id>-<slug>.md)")
	cmd.AddCommand(addCmd)
	return cmd
}

func newMergeCmd(d deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "merge <branch>",
		Short: "fast-forward-only merge a branch/worktree onto main (deterministic core)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runMerge(cwd, args[0], jsonOut, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func newCleanupCmd(d deps) *cobra.Command {
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
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runCleanupScan(cwd, jsonOut, c.OutOrStdout())
		},
	}
	scan.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(scan)
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
		&cobra.Command{
			Use:   "test-notifications",
			Short: "send a test message to every configured notification service",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runConfigTestNotifications(d)
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
		Use:   "doctor [--fix] [--pause] [--check-blocked-form <target>]",
		Short: "check the repo for common configuration issues",
		// runDoctor does its own flag parsing.
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runDoctor(args, d)
		},
	}
}

func newNotifyCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "notify <message>",
		Short: "send a message via configured Telegram/Slack notifications",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runNotify(args[0], d)
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
