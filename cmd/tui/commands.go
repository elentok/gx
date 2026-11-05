// Package tui holds the full-screen launchers: `gx worktrees`, `status`, `log`,
// `show`, `stash`, `prs` and the tab launcher `gx tickets` and `gx agents` open.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/version"

	"github.com/spf13/cobra"
)

// LogOptions configures how the log UI opens. Ref is an optional starting
// commit-ish; File, when set, is a repo-relative path the log is pre-filtered
// to (equivalent to the status "gh" mapping; follows renames).
type LogOptions struct {
	Ref  string
	File string
}

// Deps are the hooks that open each UI; tests inject fakes.
type Deps struct {
	cmdbase.Env
	RunWorktrees func(string) error
	RunStatus    func(string) error
	RunLog       func(LogOptions) error
	RunShow      func(string) error
	RunStash     func() error
	RunPRs       func(allRepos bool) error
	RunTab       func(nav.TabID) error
}

// DefaultDeps returns the production hooks, which open the real TUI. Env is
// left unset.
func DefaultDeps(dirs config.Dirs) Deps {
	return Deps{
		RunWorktrees: func(target string) error { return runWorktrees(dirs, target) },
		RunStatus:    func(target string) error { return runStatus(dirs, target) },
		RunLog:       func(opts LogOptions) error { return runLog(dirs, opts) },
		RunShow:      func(ref string) error { return runShow(dirs, ref) },
		RunStash:     func() error { return runStash(dirs) },
		RunPRs:       func(allRepos bool) error { return runPRs(dirs, allRepos) },
		RunTab:       func(tab nav.TabID) error { return RunTab(dirs, tab) },
	}
}

func getVersion() string { return version.Get() }

// NewWorktrees builds `gx worktrees`; extra are its non-UI subcommands.
func NewWorktrees(d Deps, extra ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "worktrees",
		Aliases: []string{"wt"},
		Short:   "open the worktree UI",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.RunWorktrees("")
		},
	}
	cmd.AddCommand(extra...)
	return cmd
}

func NewStatus(d Deps) *cobra.Command {
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
			return d.RunStatus(target)
		},
	}
}

func NewLog(d Deps) *cobra.Command {
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
			return d.RunLog(LogOptions{Ref: ref, File: resolved})
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "open the log pre-filtered to a file (follows renames)")
	_ = cmd.MarkFlagFilename("file")
	return cmd
}

// resolveLogFile turns a user-supplied -f path (cwd-relative or absolute) into
// the repo-relative form the log filter expects. Empty input stays empty so a
// plain `gx log` needs no repo lookup. Paths outside the worktree are rejected.
func resolveLogFile(d Deps, file string) (string, error) {
	if strings.TrimSpace(file) == "" {
		return "", nil
	}
	cwd, err := d.Getwd()
	if err != nil {
		return "", err
	}
	root, err := stage.WorktreeRoot(cwd)
	if err != nil {
		return "", err
	}
	// git reports the canonical (symlink-resolved) root; align cwd so the two
	// share a prefix before computing the repo-relative path.
	if canon, e := filepath.EvalSymlinks(cwd); e == nil {
		cwd = canon
	}
	return stage.RepoRelativePath(cwd, file, root)
}

func NewShow(d Deps) *cobra.Command {
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
			return d.RunShow(ref)
		},
	}
}

func NewStash(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "stash",
		Short: "open the stash UI",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.RunStash()
		},
	}
}

func NewPRs(d Deps) *cobra.Command {
	var allRepos bool
	cmd := &cobra.Command{
		Use:   "prs",
		Short: "open the PRs UI",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.RunPRs(allRepos)
		},
	}
	cmd.Flags().BoolVar(&allRepos, "all", false, "show outgoing PRs across all repos, not just the current one")
	return cmd
}
