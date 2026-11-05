package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/serverapi"
	"github.com/elentok/gx/cmd/ticketref"
	"github.com/elentok/gx/cmd/ticketsmigrate"
	"github.com/elentok/gx/ui/nav"

	"github.com/spf13/cobra"
)

// newEpicScopedCmd builds a cobra command that takes a single epic argument:
// cwd resolution, ticketref.ResolveEpicArg, and shell-completion via completeEpicNames
// are wired once here so epic-scoped subcommands (ensure-code-review, add,
// filter-run-log, ...) don't each hand-roll the same boilerplate. run
// receives the resolved epic path, the raw args (for any extra positional
// args beyond the epic), and the command's output writer.
func newEpicScopedCmd(d deps, use, short string, run func(epicPath string, args []string, out io.Writer) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return run(ticketref.ResolveEpicArg(d.dirs(), args[0], cwd), args, c.OutOrStdout())
		},
		ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			cwd, err := d.getwd()
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			names, err := completeEpicNames(d.dirs(), cwd)
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func newTicketsCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tickets",
		Aliases: []string{"tk"},
		Short:   "open the tickets UI",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return d.tui.RunTab(nav.TabTickets)
		},
	}
	var validateAll bool
	validateCmd := &cobra.Command{
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
	validateCmd.Flags().BoolVar(&validateAll, "all", false, "validate every project in the ticket store")
	cmd.AddCommand(validateCmd)
	var showJSON bool
	var showSection string
	showCmd := &cobra.Command{
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
	showCmd.Flags().BoolVar(&showJSON, "json", false, "emit structured JSON instead of text")
	showCmd.Flags().StringVar(&showSection, "section", "", "print only this ## section, from the ticket or from every ticket of an epic that has it")
	showCmd.MarkFlagsMutuallyExclusive("json", "section")
	cmd.AddCommand(showCmd)
	cmd.AddCommand(newTicketsLogCmd(d))
	cmd.AddCommand(newTicketsCalibrationCmd(d))
	cmd.AddCommand(newTicketsSetCmd(d))
	cmd.AddCommand(ticketsmigrate.New(ticketsmigrate.Deps{Env: d.base()}))
	cmd.AddCommand(&cobra.Command{
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
	})
	var mapsOnly bool
	epicsCmd := &cobra.Command{
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
	epicsCmd.Flags().BoolVar(&mapsOnly, "maps", false, "only print epics with ticket.md kind: map")
	cmd.AddCommand(epicsCmd)
	cmd.AddCommand(newEpicScopedCmd(d, "ensure-code-review <epic>",
		"no-op if the epic has a code-review ticket, else stamp out a stub",
		func(epicPath string, _ []string, out io.Writer) error {
			return runTicketsEnsureCodeReview(epicPath, out)
		}))
	cmd.AddCommand(newEpicScopedCmd(d, "ensure-verify <epic>",
		"no-op if the epic has a verify ticket, else stamp out a stub",
		func(epicPath string, _ []string, out io.Writer) error {
			return runTicketsEnsureVerify(epicPath, out)
		}))
	cmd.AddCommand(newTicketsPublishCmd(d))
	cmd.AddCommand(newTicketsLintClaimsCmd(d))
	cmd.AddCommand(newTicketsVerifyPlanCmd(d))
	cmd.AddCommand(&cobra.Command{
		Use:   "schema",
		Short: "print the ticket frontmatter schema (settable and read-only fields)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(c.OutOrStdout(), ticketsSchemaText)
			return err
		},
	})
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
	cmd.AddCommand(addCmd)
	followUpCmd := &cobra.Command{
		Use:   "follow-up",
		Short: "work with follow-ups, the contributor outbox",
	}
	var followUpBody string
	var followUpJSON bool
	var followUpCategory string
	followUpAddCmd := &cobra.Command{
		Use:   "add <project:epic>",
		Short: "file one follow-up (draft, research) into a follow-ups epic, enforcing the follow-up format",
		Long: "File one follow-up (draft, research) into a follow-ups epic, enforcing the follow-up format.\n\n" +
			"The body needs a category (bug or idea) and these non-empty ## sections:\n" +
			"  Source, Problem, Suggested fix\n" +
			"  Evidence (bug only)",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			epicPath, _, err := resolveAddTarget(d, args[0], "")
			if err != nil {
				return err
			}
			body, err := cmdbase.ReadBodyArg(d.stdin, followUpBody)
			if err != nil {
				return err
			}
			return runTicketsFollowUpAdd(d, epicPath, body, followUpCategory, followUpJSON, c.OutOrStdout())
		},
	}
	followUpAddCmd.Flags().StringVar(&followUpBody, "body", "-", "follow-up body, or - to read it from stdin")
	followUpAddCmd.Flags().StringVar(&followUpCategory, "category", "", "bug or idea (alternative to category: in the body frontmatter)")
	followUpAddCmd.Flags().BoolVar(&followUpJSON, "json", false, "emit {address, path} JSON")
	followUpCmd.AddCommand(followUpAddCmd)
	var followUpProject string
	followUpSearchCmd := &cobra.Command{
		Use:   "search <words...>",
		Short: "find open and archived follow-ups whose title or Problem mentions any word",
		Long: "Find open and archived follow-ups whose title or ## Problem mentions any of the words\n" +
			"(case-insensitive, OR). Prints address, status and title per hit; an archived hit also\n" +
			"shows its close reason.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runTicketsFollowUpSearch(d, followUpProject, args, c.OutOrStdout())
		},
	}
	followUpSearchCmd.Flags().StringVar(&followUpProject, "project", "", "project to search (default: the cwd's)")
	followUpCmd.AddCommand(followUpSearchCmd)
	cmd.AddCommand(followUpCmd)
	cmd.AddCommand(newTicketsEpicCmd(d))
	cmd.AddCommand(serverapi.NewAskCmd(d.serverapiDeps()))
	cmd.AddCommand(serverapi.NewAnswerCmd(d.serverapiDeps(), false))
	var sectionJSON, sectionClear bool
	sectionCmd := &cobra.Command{
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
	sectionCmd.Flags().BoolVar(&sectionJSON, "json", false, "emit {address, path} JSON")
	sectionCmd.Flags().BoolVar(&sectionClear, "clear", false, "blank the section (takes no content)")
	// Content is free text that often starts with "-" (a markdown list), so
	// flags must come before the positional args.
	sectionCmd.Flags().SetInterspersed(false)
	cmd.AddCommand(sectionCmd)
	var moveSlug string
	var moveDryRun, moveJSON bool
	moveCmd := &cobra.Command{
		Use:   "move <addr> <epic>",
		Short: "move one flat, never-run draft/open ticket to another epic of the same project",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			return runTicketsMove(d, args[0], args[1], moveSlug, moveDryRun, moveJSON, c.OutOrStdout())
		},
	}
	moveCmd.Flags().StringVar(&moveSlug, "slug", "", "filename slug in the target epic (default: the ticket's current slug)")
	moveCmd.Flags().BoolVar(&moveDryRun, "dry-run", false, "print the plan, write nothing")
	moveCmd.Flags().BoolVar(&moveJSON, "json", false, "emit {from, to, path, rewritten} JSON")
	cmd.AddCommand(moveCmd)
	cmd.AddCommand(newTicketsCloseCmd(d))
	var filterTicket string
	var filterEvents []string
	filterRunLogCmd := newEpicScopedCmd(d, "filter-run-log <epic>",
		"print an epic's run-log.jsonl events, filtered by ticket/event type",
		func(epicPath string, _ []string, out io.Writer) error {
			return runTicketsFilterRunLog(epicPath, filterTicket, filterEvents, out)
		})
	filterRunLogCmd.Flags().StringVar(&filterTicket, "ticket", "", "only events for this ticket id")
	filterRunLogCmd.Flags().StringArrayVar(&filterEvents, "event", nil, "only events of this type (repeatable)")
	cmd.AddCommand(filterRunLogCmd)
	return cmd
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
