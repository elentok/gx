package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

// ticketsSchemaText is gx tickets schema's entire output, verbatim from
// .scratch/ralph-tickets-visibility/issues/02-tickets-set-cli.md's Answer:
// plain text an agent skill can quote directly as instructions, not a format
// meant to be parsed programmatically.
const ticketsSchemaText = `Ticket frontmatter fields:

Settable fields:
  status (enum, --status): open, claimed, ready-for-agent, ready-for-human, needs-triage,
    needs-info, needs-attention, done
  blocked_by (comma-separated ticket IDs, --blocked-by): e.g. 01,03
  children (comma-separated ticket IDs, --children): tickets this one produced (a
    mid-flight fork, or the fix tickets a code-review ticket opened)
  parent (ticket ID, --parent): the ticket this one was produced from
  type (enum, --type): task, research, prototype, grilling, code-review
  expected_context_window (non-negative int, --expected-context-window)
  commitless (bool, --commitless): true/false. Set true when you intentionally finish an
    iteration with no commit (e.g. exploration concluded no code change was warranted) —
    pair it with a status that doesn't leave the ticket claimed (done, ready-for-human,
    needs-triage), or it's still treated as an unresolved iteration.

Read-only fields (gx-managed, not settable via ` + "`set`" + `):
  id — ticket identity, fixed at creation
  actual_context_window — stamped by ralphloop at land time
  elapsed_time — stamped by ralphloop at land time

Epic frontmatter fields:

An epic's optional ` + "`.scratch/<epic>/epic.yaml`" + ` sidecar file holds epic-level timing,
distinct from any ticket's own frontmatter. Both fields are gx-managed, not settable via
` + "`tickets set`" + `, and an epic with no epic.yaml yet has both unset.
  started_at (RFC3339 timestamp) — when work on the epic began
  completed_at (RFC3339 timestamp) — when the epic's last ticket landed
`

func newTicketsSetCmd() *cobra.Command {
	var (
		status                string
		blockedBy             string
		children              string
		parent                string
		ticketType            string
		expectedContextWindow string
		commitless            string
		force                 bool
	)

	cmd := &cobra.Command{
		Use:   "set <path>",
		Short: "validated, sparse frontmatter writes to a ticket file",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runTicketsSet(c, args[0], c.OutOrStdout(), c.ErrOrStderr())
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "set the status field")
	cmd.Flags().StringVar(&blockedBy, "blocked-by", "", "set blocked_by (comma-separated ticket IDs)")
	cmd.Flags().StringVar(&children, "children", "", "set children (comma-separated ticket IDs)")
	cmd.Flags().StringVar(&parent, "parent", "", "set parent")
	cmd.Flags().StringVar(&ticketType, "type", "", "set the type field")
	cmd.Flags().StringVar(&expectedContextWindow, "expected-context-window", "", "set expected_context_window")
	cmd.Flags().StringVar(&commitless, "commitless", "", "set commitless (true/false)")
	cmd.Flags().BoolVar(&force, "force", false, "allow --status done despite unresolved blocked_by")

	return cmd
}

// ticketSetField is one entry in the ordered flag-to-field table
// runTicketsSet drives: the flag's name, and how to apply its (string) value
// to a *schema.Ticket. Keeping the table ordered (rather than ranging over
// cmd.Flags()) makes the "updated (field=value, ...)" summary deterministic.
type ticketSetField struct {
	flag  string
	field string // YAML field name, for the success summary
	apply func(t *schema.Ticket, value string)
}

var ticketSetFields = []ticketSetField{
	{"status", "status", func(t *schema.Ticket, v string) { t.Status = schema.Status(v) }},
	{"blocked-by", "blocked_by", func(t *schema.Ticket, v string) { t.BlockedBy = parseCSVIDs(v) }},
	{"children", "children", func(t *schema.Ticket, v string) { t.Children = parseCSVIDs(v) }},
	{"parent", "parent", func(t *schema.Ticket, v string) { t.Parent = parseIDPtr(v) }},
	{"type", "type", func(t *schema.Ticket, v string) { t.Type = schema.TicketType(v) }},
	{"expected-context-window", "expected_context_window", func(t *schema.Ticket, v string) {
		n, _ := strconv.Atoi(v)
		t.ExpectedContextWindow = n
	}},
	{"commitless", "commitless", func(t *schema.Ticket, v string) {
		b, _ := strconv.ParseBool(v)
		t.Commitless = b
	}},
}

// parseIDPtr returns nil for an empty value (clearing the field), or a
// pointer to the parsed TicketID otherwise.
func parseIDPtr(value string) *schema.TicketID {
	if value == "" {
		return nil
	}
	id := schema.TicketID(value)
	return &id
}

// parseCSVIDs splits a comma-separated flag value into TicketIDs. An
// explicitly passed empty string clears the field: strings.Split("", ",")
// would otherwise yield a single empty-string element, so that case is
// special-cased to nil.
func parseCSVIDs(value string) []schema.TicketID {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	ids := make([]schema.TicketID, len(parts))
	for i, p := range parts {
		ids[i] = schema.TicketID(p)
	}
	return ids
}

// runTicketsSet applies every flag actually passed on c to path's ticket via
// schema.UpdateTicket, then prints a summary of just the fields changed this
// call. Flags never passed leave their Ticket field exactly as parsed.
func runTicketsSet(c *cobra.Command, path string, w, stderr io.Writer) error {
	if c.Flags().Changed("status") {
		status, _ := c.Flags().GetString("status")
		if schema.Status(status) == schema.StatusDone {
			force, _ := c.Flags().GetBool("force")
			if err := checkBlockersBeforeDone(path, force, stderr); err != nil {
				return err
			}
		}
	}

	var changed []string

	err := schema.UpdateTicket(path, func(t *schema.Ticket) {
		for _, f := range ticketSetFields {
			if !c.Flags().Changed(f.flag) {
				continue
			}
			value, _ := c.Flags().GetString(f.flag)
			f.apply(t, value)
			changed = append(changed, fmt.Sprintf("%s=%s", f.field, value))
		}
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "%s: updated (%s)\n", path, strings.Join(changed, ", "))
	return nil
}

// checkBlockersBeforeDone refuses a --status done write for a ticket whose
// own blocked_by isn't actually resolved, unless force is set — closing the
// gap where an agent could mark a ticket done without ever going through the
// scheduler's own claim-time blocker check (ralphloop's claimNext), which is
// how a mid-flight-fork placeholder can be born already-done with an
// unverified blocker (see gx-investigate/gotchas.md and
// tickets/status.go's FullyDone doc comment). Only enforced for tickets
// living under the tracker's <epic>/issues/<file>.md layout — anything else
// (ad-hoc files) is left unchecked, since there's no epic to resolve
// blockers against.
func checkBlockersBeforeDone(path string, force bool, stderr io.Writer) error {
	issuesDir := filepath.Dir(path)
	if filepath.Base(issuesDir) != "issues" {
		return nil
	}
	epicPath := filepath.Dir(issuesDir)

	epic, unlock, err := tickets.LoadLockedEpic(epicPath)
	if err != nil {
		return fmt.Errorf("checking blockers before marking done: %w", err)
	}
	defer unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	var target *tickets.Ticket
	for i := range epic.Tickets {
		if tp, err := filepath.Abs(epic.Tickets[i].Path); err == nil && tp == absPath {
			target = &epic.Tickets[i]
			break
		}
	}
	if target == nil {
		return nil
	}

	unresolved := epic.UnresolvedBlockers(*target)
	if len(unresolved) == 0 {
		return nil
	}

	if !force {
		return fmt.Errorf(
			"%s has unresolved blocked_by (%s); refusing to mark done without --force",
			path, strings.Join(unresolved, ", "),
		)
	}

	fmt.Fprintf(stderr,
		"warning: %s forced done with unresolved blocked_by (%s) — anything blocked on it will trust this status without the blocker having actually finished\n",
		path, strings.Join(unresolved, ", "),
	)
	return nil
}
