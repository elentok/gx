package cmd

import (
	"io"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/ticketref"

	"github.com/spf13/cobra"
)

// ticketsFlowCommands returns the flow-side verbs of `gx tickets`: epic,
// publish, follow-up, the ensure verbs, verify-plan, calibration, lint-claims
// and filter-run-log.
func ticketsFlowCommands(d deps) []*cobra.Command {
	return []*cobra.Command{
		newTicketsCalibrationCmd(d),
		ticketref.NewEpicScopedCmd(d.base(), "ensure-code-review <epic>",
			"no-op if the epic has a code-review ticket, else stamp out a stub",
			func(epicPath string, _ []string, out io.Writer) error {
				return runTicketsEnsureCodeReview(epicPath, out)
			}),
		ticketref.NewEpicScopedCmd(d.base(), "ensure-verify <epic>",
			"no-op if the epic has a verify ticket, else stamp out a stub",
			func(epicPath string, _ []string, out io.Writer) error {
				return runTicketsEnsureVerify(epicPath, out)
			}),
		newTicketsPublishCmd(d),
		newTicketsLintClaimsCmd(d),
		newTicketsVerifyPlanCmd(d),
		newTicketsFollowUpCmd(d),
		newTicketsEpicCmd(d),
		newTicketsFilterRunLogCmd(d),
	}
}

func newTicketsFollowUpCmd(d deps) *cobra.Command {
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
			epicPath, _, err := ticketref.ResolveAddTarget(d.base(), args[0], "")
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
	return followUpCmd
}

func newTicketsFilterRunLogCmd(d deps) *cobra.Command {
	var filterTicket string
	var filterEvents []string
	cmd := ticketref.NewEpicScopedCmd(d.base(), "filter-run-log <epic>",
		"print an epic's run-log.jsonl events, filtered by ticket/event type",
		func(epicPath string, _ []string, out io.Writer) error {
			return runTicketsFilterRunLog(epicPath, filterTicket, filterEvents, out)
		})
	cmd.Flags().StringVar(&filterTicket, "ticket", "", "only events for this ticket id")
	cmd.Flags().StringArrayVar(&filterEvents, "event", nil, "only events of this type (repeatable)")
	return cmd
}
