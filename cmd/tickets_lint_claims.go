package cmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func newTicketsLintClaimsCmd(d deps) *cobra.Command {
	var manifest string
	c := &cobra.Command{
		Use:   "lint-claims <project:epic> [--manifest <file>]",
		Short: "check that the commands, paths, tests and flows a ticket quotes exist",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			report, err := lintEpicClaims(d, cwd, resolveEpicArg(d.dirs(), args[0], cwd), manifest, c.OutOrStdout())
			if err != nil {
				return err
			}
			if report.Refused() {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	c.Flags().StringVar(&manifest, "manifest", "", "lint the tickets of this publish manifest instead of the epic's")
	return c
}

// lintEpicClaims lints the epic's open tickets, or the manifest's entries when
// manifestPath is set, against the checkout at cwd, and prints the report.
func lintEpicClaims(d deps, cwd, epicPath, manifestPath string, w io.Writer) (tickets.LintReport, error) {
	root, err := git.Toplevel(cwd)
	if err != nil {
		return tickets.LintReport{}, err
	}
	commit, err := git.ShortHead(root)
	if err != nil {
		return tickets.LintReport{}, err
	}
	epic := tickets.LoadEpicAt(epicPath)
	var existing, entries []tickets.LintEntry
	var ids []string
	for _, t := range epic.Tickets {
		ids = append(ids, t.DisplayNumber())
		e := tickets.LintEntry{Name: t.DisplayNumber(), Type: t.Type, Body: t.Body, BlockedBy: t.BlockedBy}
		existing = append(existing, e)
		if manifestPath == "" && t.Status != string(schema.StatusDone) {
			entries = append(entries, e)
		}
	}
	if manifestPath != "" {
		m, err := tickets.ParseManifest(manifestPath, ids)
		if err != nil {
			return tickets.LintReport{}, err
		}
		for _, e := range m.Entries {
			entries = append(entries, tickets.LintEntry{Name: e.Slug, Type: e.Type, Body: e.Body, BlockedBy: e.BlockedBy})
		}
	}

	env := tickets.LintEnv{
		Root:    root,
		Command: commandChecker(newRootCmd(d)),
		GoTests: tickets.LoadGoTestNames(root),
		Flows:   tickets.LoadFlowCatalog(root),
		TopDirs: topLevelDirs(root),
	}
	report := tickets.LintClaims(entries, existing, env)
	printLintReport(w, commit, report)
	return report, nil
}

func topLevelDirs(root string) []string {
	entries, _ := os.ReadDir(root)
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

var subcommandWordRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// commandChecker walks gx's command tree. A word that is not a subcommand is an
// argument unless it looks like a subcommand name, so `gx tickets show a/01` is
// fine and `gx tickets nosuch` is not.
func commandChecker(root *cobra.Command) func(words, flags []string) []tickets.CommandProblem {
	return func(words, flags []string) []tickets.CommandProblem {
		cur := root
		for i, w := range words {
			next := findSubcommand(cur, w)
			if next == nil {
				if len(cur.Commands()) > 0 && subcommandWordRe.MatchString(w) {
					return []tickets.CommandProblem{{Words: words[:i+1], Text: fmt.Sprintf("no such command %q", w)}}
				}
				break
			}
			cur = next
		}
		var out []tickets.CommandProblem
		for _, f := range flags {
			name := strings.TrimPrefix(f, "--")
			if name != "help" && !flagDefined(cur, name) {
				out = append(out, tickets.CommandProblem{Words: words, Flag: f, Text: fmt.Sprintf("no such flag %s", f)})
			}
		}
		return out
	}
}

func findSubcommand(c *cobra.Command, name string) *cobra.Command {
	for _, s := range c.Commands() {
		if s.Name() == name || slices.Contains(s.Aliases, name) {
			return s
		}
	}
	return nil
}

func flagDefined(c *cobra.Command, name string) bool {
	for p := c; p != nil; p = p.Parent() {
		if p.Flags().Lookup(name) != nil || p.PersistentFlags().Lookup(name) != nil {
			return true
		}
	}
	return false
}

func printLintReport(w io.Writer, commit string, r tickets.LintReport) {
	fmt.Fprintf(w, "lint-claims: checked the current checkout's working tree at %s\n", commit)
	for _, f := range r.Misses {
		fmt.Fprintf(w, "miss: %s: %s\n", f.Entry, f.Text)
	}
	for _, f := range r.Warnings {
		fmt.Fprintf(w, "warning: %s: %s\n", f.Entry, f.Text)
	}
	for _, f := range r.Planned {
		fmt.Fprintf(w, "planned: %s: %s\n", f.Entry, f.Text)
	}
	for _, s := range r.Skipped {
		fmt.Fprintf(w, "skipped: %s\n", s)
	}
}
