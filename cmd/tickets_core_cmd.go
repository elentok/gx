package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/ticketref"

	"github.com/spf13/cobra"
)

func newTicketsValidateCmd(d deps) *cobra.Command {
	var validateAll bool
	cmd := &cobra.Command{
		Use:   "validate <addr|path> | --all",
		Short: "validate a ticket and its whole project, by address or file path; --all walks the store",
		Args: func(_ *cobra.Command, args []string) error {
			if validateAll {
				return cobra.NoArgs(nil, args)
			}
			return cobra.ExactArgs(1)(nil, args)
		},
		RunE: func(c *cobra.Command, args []string) error {
			if validateAll {
				cfg, err := d.dirs().Load()
				if err != nil {
					return err
				}
				return runTicketsValidateAll(cfg.TicketStore.Path, c.OutOrStdout(), c.ErrOrStderr())
			}
			path, err := ticketref.ResolveTicketRef(d.dirs(), d.getwd, args[0])
			if err != nil {
				return err
			}
			return runTicketsValidate(path, c.OutOrStdout(), c.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&validateAll, "all", false, "validate every project in the ticket store")
	return cmd
}

func newTicketsShowCmd(d deps) *cobra.Command {
	var showJSON bool
	var showSection string
	cmd := &cobra.Command{
		Use: "show <addr>",
		// Agents keep guessing `gx tickets list <epic>`.
		Aliases: []string{"list"},
		Short:   "print a ticket (project:epic/06, epic/06 or 06) or an epic and its tickets (project:epic or epic)",
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsShow(d.dirs(), cwd, args[0], showJSON, showSection, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&showJSON, "json", false, "emit structured JSON instead of text")
	cmd.Flags().StringVar(&showSection, "section", "", "print only this ## section, from the ticket or from every ticket of an epic that has it")
	cmd.MarkFlagsMutuallyExclusive("json", "section")
	return cmd
}

func newTicketsRootCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "root",
		Short: "print the ticket root (in the ticket store, or .scratch before migrate) for the current repo",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsRoot(d.dirs(), cwd, c.OutOrStdout())
		},
	}
}

func newTicketsEpicsCmd(d deps) *cobra.Command {
	var mapsOnly bool
	cmd := &cobra.Command{
		Use:   "epics",
		Short: "print bare epic slugs under the current repo's ticket root",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsEpics(d.dirs(), cwd, c.OutOrStdout(), mapsOnly)
		},
	}
	cmd.Flags().BoolVar(&mapsOnly, "maps", false, "only print epics with ticket.md kind: map")
	return cmd
}

func newTicketsSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "print the ticket frontmatter schema (settable and read-only fields)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(c.OutOrStdout(), ticketsSchemaText)
			return err
		},
	}
}

func newTicketsAddCmd(d deps) *cobra.Command {
	var addParent, addSlug, addBody string
	var addJSON bool
	var addCmd *cobra.Command
	addCmd = newEpicScopedCmd(d, "add <epic|parent-addr>",
		"atomically allocate the next ticket ID and stamp out a ticket (a draft stub, or open with --body)",
		func(arg string, _ []string, out io.Writer) error {
			flags, err := fieldFlags(addCmd.Flags())
			if err != nil {
				return err
			}
			epicPath, parent, err := resolveAddTarget(d, arg, addParent)
			if err != nil {
				return err
			}
			if addBody == "" {
				if len(flags.set) > 0 {
					return fmt.Errorf("field flags need --body; fill a stub's fields in with `tickets set`")
				}
				return runTicketsAdd(d, epicPath, parent, addSlug, out)
			}
			body, err := cmdbase.ReadBodyArg(d.stdin, addBody)
			if err != nil {
				return err
			}
			return runTicketsAddBody(d, epicPath, parent, addSlug, body, flags, addJSON, out, d.stderr)
		})
	addCmd.Flags().String("blocked-by", "", "with --body, set blocked_by (comma-separated ticket IDs; omitted means none)")
	addCmd.Flags().String("type", "", "with --body, set the type field (default implement)")
	addCmd.Flags().String("expected-context-window", "", "with --body, set expected_context_window")
	addCmd.Flags().String("commitless", "", "with --body, set commitless (true/false)")
	addCmd.Flags().String("category", "", "with --body, set category (bug or idea)")
	addCmd.Flags().StringVar(&addParent, "parent", "", "allocate a lettered child of this ticket ID (or, if parent is itself lettered, one numeric level past it)")
	addCmd.Flags().StringVar(&addSlug, "slug", "", "descriptive filename slug, e.g. \"wire-tree-model-selection\" (required; the stub is written to <id>-<slug>.md)")
	addCmd.Flags().StringVar(&addBody, "body", "", "ticket body, or - to read it from stdin; the ticket is written open and printed by address")
	addCmd.Flags().BoolVar(&addJSON, "json", false, "with --body, emit {address, path} JSON")
	return addCmd
}

func newTicketsSectionCmd(d deps) *cobra.Command {
	var sectionJSON, sectionClear bool
	cmd := &cobra.Command{
		Use:   "section <addr|path> <heading> [<content|->]",
		Short: "print (2 args) or replace/append (3 args) one ## section of a ticket's body; content from the argument or stdin (-)",
		Long: "With <addr> and <heading> only, prints the section (same as show --section).\n" +
			"With content, replaces or appends the section; empty or whitespace-only content is refused.\n" +
			"To blank a section on purpose: gx tickets section --clear <addr> <heading>.",
		Args: func(_ *cobra.Command, args []string) error {
			if sectionClear {
				return cobra.ExactArgs(2)(nil, args)
			}
			return cobra.RangeArgs(2, 3)(nil, args)
		},
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 2 && !sectionClear {
				cwd, err := d.getwd()
				if err != nil {
					return err
				}
				return runTicketsShow(d.dirs(), cwd, args[0], false, args[1], c.OutOrStdout())
			}
			path, err := ticketref.ResolveTicketRef(d.dirs(), d.getwd, args[0])
			if err != nil {
				return err
			}
			content := ""
			if !sectionClear {
				if content, err = cmdbase.ReadBodyArg(d.stdin, args[2]); err != nil {
					return err
				}
			}
			return runTicketsSection(d, path, args[1], content, sectionClear, sectionJSON, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&sectionJSON, "json", false, "emit {address, path} JSON")
	cmd.Flags().BoolVar(&sectionClear, "clear", false, "blank the section (takes no content)")
	// Content is free text that often starts with "-" (a markdown list), so
	// flags must come before the positional args.
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func newTicketsMoveCmd(d deps) *cobra.Command {
	var moveSlug string
	var moveDryRun, moveJSON bool
	cmd := &cobra.Command{
		Use:   "move <addr> <epic>",
		Short: "move one flat, never-run draft/open ticket to another epic of the same project",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			return runTicketsMove(d, args[0], args[1], moveSlug, moveDryRun, moveJSON, c.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&moveSlug, "slug", "", "filename slug in the target epic (default: the ticket's current slug)")
	cmd.Flags().BoolVar(&moveDryRun, "dry-run", false, "print the plan, write nothing")
	cmd.Flags().BoolVar(&moveJSON, "json", false, "emit {from, to, path, rewritten} JSON")
	return cmd
}
