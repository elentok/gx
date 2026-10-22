package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/repair"
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
  status (enum, --status): draft, open. Orchestration statuses (claimed, done,
    paused, cancelled) are the server's alone and refused here — gx
    claims, pauses and merges a ticket itself. Exception: in a map epic (ticket.md
    kind: map), claimed and done are settable, since nothing schedules a map.
    Required on every ticket. draft is held-back work: it never enters an epic's
    frontier, so no agent is ever handed it.
  blocked_by (comma-separated ticket IDs, --blocked-by): e.g. 01,03. A bare ID names a ticket in
    this epic; epic/06 (or project:epic/06 for this project) names one in another epic of the
    project. To wait on another epic's work, block on it this way — never pause the ticket as a
    stand-in. Dangling refs, another project's prefix and cycles are refused before writing.
  parent (ticket ID, --parent): the ticket this one was produced from. Must name an
    existing ticket in the same epic, and may not point into this ticket's own fork
    subtree (that would make the fork graph cyclic) — both are rejected.
  type (enum, --type): task, research, prototype, grilling, code-review
  expected_context_window (non-negative int, --expected-context-window)
  commitless (bool, --commitless): true/false. Set true when you intentionally finish an
    iteration with no commit (e.g. exploration concluded no code change was warranted) —
    pair it with a status that doesn't leave the ticket claimed (done),
    or it's still treated as an unresolved iteration.

Agent-only settable fields:
  iteration_status (enum, --iteration-status): working, asking, finished. The
    agent's own self-report for the current claim, distinct from status. This report
    can only start a merge, never conclude one — gx's own commit count and
    cherry-pick decide whether the ticket is actually merged. Never pair finished with
    --status=done: the merge status is gx's alone to write.

Read-only fields (gx-managed, not settable via ` + "`set`" + `):
  id — ticket identity, fixed at creation
  actual_context_window — stamped by ralphloop at merge time
  elapsed_time — stamped by ralphloop at merge time
  mutes — notification throttles tripped for this ticket (event type + timestamp),
    machine-written only
  iteration_finished_at (RFC3339 timestamp) — stamped when an agent reports
    --iteration-status finished, cleared otherwise
  claimed_at (RFC3339 timestamp) — stamped on every move into claimed, including a
    re-claim after a start-over or resume; cleared with done_at
  done_at (RFC3339 timestamp) — stamped when the ticket moves into done, cleared by
    a new claim
  exported_at (RFC3339 timestamp) — when gx exported the ticket

Other frontmatter fields:
  category (enum): bug, idea. Optional on any ticket.
  agent (string): the agent that runs the ticket (claude, codex). Read at claim; a missing
    or unknown value runs the project's default agent. Written when the ticket is created.
  type evolve: a commitless, top-level-only ticket (it may not have a parent).

Epic frontmatter fields:

An epic's ticket.md may set kind: map to mark a wayfinder map epic: hand-driven,
never scheduled by the server.

` + "`tickets set project:epic`" + ` (or ` + "`tickets epic set`" + `) writes an epic's kind (map, loop), status
(draft, open), provisional_destination and blocked_by (whole-epic tokens such as other or
project:other: the whole epic waits on them, checked like a ticket's); created_at, completed_at, merged_at, evolved_at, evolve_skipped and
evolve_ticket are refused (server-stamped). ` + "`tickets section project:epic`" + ` writes a body section of its ticket.md.

An epic's ticket.md also holds epic-level timing. These fields are gx-managed, not
settable via ` + "`tickets set`" + `, and unset until gx stamps them.
  started_at (RFC3339 timestamp) — when work on the epic began
  created_at (RFC3339 timestamp) — when the epic was created
  completed_at (RFC3339 timestamp) — when the epic's last ticket merged (a map: when its
    <map>-impl epic was created)
  merged_at (RFC3339 timestamp) — when the epic's branch merged into its target
    (stamped by the server or ` + "`gx merge`" + `); a merged epic is one with merged_at set

Evolve fields (optional, validated):
  evolved_at (RFC3339 timestamp), evolve_skipped (bool), evolve_ticket (address),
  provisional_destination (bool)
`

func newTicketsSetCmd(d deps) *cobra.Command {
	var (
		status                string
		blockedBy             string
		parent                string
		ticketType            string
		expectedContextWindow string
		commitless            string
		iterationStatus       string
	)

	cmd := &cobra.Command{
		Use:   "set <addr|path>",
		Short: "validated, sparse frontmatter writes to a ticket, by address or file path",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path, err := resolveTicketRef(d.getwd, args[0])
			if err != nil {
				return err
			}
			return runTicketsSet(c, path, c.OutOrStdout(), c.ErrOrStderr(), d.getwd)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "set the status field")
	cmd.Flags().StringVar(&blockedBy, "blocked-by", "", "set blocked_by (comma-separated ticket IDs)")
	cmd.Flags().StringVar(&parent, "parent", "", "set parent")
	cmd.Flags().StringVar(&ticketType, "type", "", "set the type field")
	cmd.Flags().StringVar(&expectedContextWindow, "expected-context-window", "", "set expected_context_window")
	cmd.Flags().StringVar(&commitless, "commitless", "", "set commitless (true/false)")
	cmd.Flags().StringVar(&iterationStatus, "iteration-status", "", "set the iteration_status field (working, asking, finished)")

	// Declared only so a write attempt is refused with a clear message rather
	// than cobra's generic unknown-flag error.
	cmd.Flags().String("exported-at", "", "refused: exported_at is gx-managed")
	_ = cmd.Flags().MarkHidden("exported-at")

	// Epic address only (project:epic): the settable epic fields.
	cmd.Flags().String("kind", "", "epic only: set kind (map or loop)")
	cmd.Flags().String("provisional-destination", "", "epic only: set provisional_destination (true/false)")
	for _, field := range tickets.EpicServerStamped {
		flag := strings.ReplaceAll(field, "_", "-")
		cmd.Flags().String(flag, "", "refused: "+field+" is server-stamped")
		_ = cmd.Flags().MarkHidden(flag)
	}

	cmd.Flags().Bool("json", false, "emit {address, path, updated, via, actor} JSON instead of text")

	return cmd
}

// runEpicSet is runTicketsSet for an epic's ticket.md, validated against the
// epic schema rather than the ticket one. A server-stamped field refuses with
// tickets.CodeServerStamped before anything is written.
func runEpicSet(c *cobra.Command, path string, w io.Writer, getwd func() (string, error)) error {
	for _, field := range tickets.EpicServerStamped {
		if c.Flags().Changed(strings.ReplaceAll(field, "_", "-")) {
			return &tickets.AddressError{Code: tickets.CodeServerStamped, Msg: fmt.Sprintf("%s: %s is server-stamped and not settable via `tickets set`", ticketLabel(path), field)}
		}
	}

	var (
		update  tickets.EpicUpdate
		changed []string
	)
	if c.Flags().Changed("kind") {
		v, _ := c.Flags().GetString("kind")
		update.Kind = &v
		changed = append(changed, "kind="+v)
	}
	if c.Flags().Changed("status") {
		v, _ := c.Flags().GetString("status")
		update.Status = &v
		changed = append(changed, "status="+v)
	}
	if c.Flags().Changed("provisional-destination") {
		v, _ := c.Flags().GetString("provisional-destination")
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("%s: --provisional-destination %q: want true or false", ticketLabel(path), v)
		}
		update.ProvisionalDestination = &b
		changed = append(changed, "provisional_destination="+v)
	}
	var notices []string
	if c.Flags().Changed("blocked-by") {
		v, _ := c.Flags().GetString("blocked-by")
		refs := []string{}
		if v != "" {
			refs = strings.Split(v, ",")
		}
		var err error
		if notices, err = tickets.CheckEpicBlockedByWrite(path, refs); err != nil {
			return fmt.Errorf("%s: rejecting --blocked-by %s: %w", ticketLabel(path), v, err)
		}
		update.BlockedBy = &refs
		changed = append(changed, "blocked_by="+v)
	}
	if len(changed) == 0 {
		return fmt.Errorf("%s: an epic takes --kind, --status, --provisional-destination and --blocked-by", ticketLabel(path))
	}
	if err := tickets.UpdateEpicFields(path, update); err != nil {
		return fmt.Errorf("%s: %w", ticketLabel(path), err)
	}
	printNotices(c.ErrOrStderr(), notices)

	if jsonOut, _ := c.Flags().GetBool("json"); jsonOut {
		return writeStamped(w, struct {
			Address string   `json:"address"`
			Path    string   `json:"path"`
			Updated []string `json:"updated"`
		}{ticketLabel(path), path, changed}, viaDirect, callerActor(getwd))
	}
	fmt.Fprintf(w, "%s: updated (%s)\n", ticketLabel(path), strings.Join(changed, ", "))
	return nil
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
	{"iteration-status", "iteration_status", func(t *schema.Ticket, v string) { t.IterationStatus = schema.IterationStatus(v) }},
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

// orchestrationStatuses are the statuses `set` no longer writes: claims, pauses,
// merging and cancellation belong to the server's orchestration path.
var orchestrationStatuses = map[schema.Status]bool{
	schema.StatusClaimed:      true,
	schema.StatusDone:         true,
	schema.StatusLegacyAnswer: true,
	schema.StatusLegacyRepair: true,
	schema.StatusPaused:       true,
	schema.StatusCancelled:    true,
}

// mapEpicStatuses are the orchestration statuses `set` does write for a ticket
// in a map epic: nothing schedules a map, so a person claims and closes its
// decision tickets by hand.
var mapEpicStatuses = map[schema.Status]bool{
	schema.StatusClaimed: true,
	schema.StatusDone:    true,
}

// inMapEpic reports whether path is a ticket under <epic>/issues/ of a map epic.
func inMapEpic(path string) bool {
	issuesDir := filepath.Dir(path)
	return filepath.Base(issuesDir) == "issues" && tickets.IsMapEpic(filepath.Dir(issuesDir))
}

// runTicketsSet applies every flag actually passed on c to path's ticket via
// schema.UpdateTicket, then prints a summary of just the fields changed this
// call. Flags never passed leave their Ticket field exactly as parsed.
func runTicketsSet(c *cobra.Command, path string, w, stderr io.Writer, getwd func() (string, error)) error {
	if isEpicTicketMD(path) {
		return runEpicSet(c, path, w, getwd)
	}
	if c.Flags().Changed("exported-at") {
		return fmt.Errorf("%s: exported_at is gx-managed and not settable via `tickets set`", path)
	}

	if c.Flags().Changed("status") {
		status, _ := c.Flags().GetString("status")
		s := schema.Status(status)
		if orchestrationStatuses[s] && !(mapEpicStatuses[s] && inMapEpic(path)) {
			return fmt.Errorf("%s: --status %s is an orchestration status; the server owns it, `tickets set` only moves a ticket between draft and open (or, in a kind: map epic, claims and closes it)", path, status)
		}
		if err := checkAgentStatusGuard(path, schema.Status(status), getwd); err != nil {
			return err
		}
		if schema.Status(status) == schema.StatusOpen {
			if err := checkBodyBeforeOpen(path); err != nil {
				return err
			}
		}
	}

	if c.Flags().Changed("iteration-status") {
		if err := checkIterationStatusFinishedGuard(c, path); err != nil {
			return err
		}
	}
	if err := checkOpenQuestionGuard(c, path); err != nil {
		return err
	}

	if c.Flags().Changed("parent") {
		parent, _ := c.Flags().GetString("parent")
		unlock, err := lockEpicForParentWrite(path, parent)
		if err != nil {
			return err
		}
		if unlock != nil {
			// Held across the write below on purpose: validating and
			// releasing first would let two agents re-parenting at once each
			// validate against their own snapshot and jointly close a cycle
			// neither of them saw.
			defer unlock()
		}
	}

	var notices []string
	if c.Flags().Changed("blocked-by") {
		current, err := schema.ParseTicket(path)
		if err != nil {
			return err
		}
		blockedBy, _ := c.Flags().GetString("blocked-by")
		current.BlockedBy = parseCSVIDs(blockedBy)
		if notices, err = tickets.CheckBlockedByWrite(path, current); err != nil {
			return fmt.Errorf("%s: rejecting --blocked-by %s: %w", ticketLabel(path), blockedBy, err)
		}
	}

	var changed []string

	err := schema.UpdateTicket(path, func(t *schema.Ticket) {
		wasCommitless := t.IsCommitless()
		defer func() {
			// A fork closes its commitful placeholder as commitless; base is
			// meaningless then and validation rejects the pair. Only the
			// transition clears it, so authoring-time rejection is unchanged.
			if !wasCommitless && t.IsCommitless() {
				t.Base = ""
			}
		}()
		for _, f := range ticketSetFields {
			if !c.Flags().Changed(f.flag) {
				continue
			}
			value, _ := c.Flags().GetString(f.flag)
			f.apply(t, value)
			changed = append(changed, fmt.Sprintf("%s=%s", f.field, value))
		}
		if c.Flags().Changed("iteration-status") {
			t.IterationFinishedAt = time.Time{}
			if t.IterationStatus == schema.IterationStatusFinished {
				t.IterationFinishedAt = time.Now().UTC()
			}
		}
	})
	if err != nil {
		return err
	}
	printNotices(stderr, notices)

	pingServer(path)
	if jsonOut, _ := c.Flags().GetBool("json"); jsonOut {
		return writeStamped(w, struct {
			Address string   `json:"address"`
			Path    string   `json:"path"`
			Updated []string `json:"updated"`
		}{ticketLabel(path), path, changed}, viaDirect, callerActor(getwd))
	}
	fmt.Fprintf(w, "%s: updated (%s)\n", ticketLabel(path), strings.Join(changed, ", "))
	return nil
}

// checkAgentStatusGuard is the merge-decision guard rail: it refuses a
// --status write from an iteration agent's own worktree unless it's the one
// promotion mid-flight forking depends on, draft -> open. The guard is keyed
// off the caller's git branch rather than a flag or env var — see
// .scratch/no-silent-stalls/issues/11-cli-status-guard.md for why branch is
// the accident-resistant signal here. It is a guard rail, not a boundary: an
// agent that changes directory out of its worktree evades it as easily as it
// could alias a flag away, and that's fine — the failure mode being defended
// against is a stale instruction, not an adversary. getwd failing, or the
// branch not matching "ralph-loop/*", both mean no guard: unrecognised is
// never treated as a false refusal, so hand-driven epics keep working
// exactly as they do today.
func checkAgentStatusGuard(path string, newStatus schema.Status, getwd func() (string, error)) error {
	branch, ok := isRalphLoopBranch(getwd)
	if !ok {
		return nil
	}

	if newStatus == schema.StatusOpen {
		current, err := schema.ParseTicket(path)
		if err == nil && current.Status == schema.StatusDraft {
			return nil
		}
	}

	return fmt.Errorf(
		"%s: --status %s refused from a ralph-loop/* branch (%s); an iteration agent may only promote a draft ticket to open, the merge status is gx's to set",
		path, newStatus, branch,
	)
}

// lockEpicForTicket loads the epic owning path under the epic's allocation
// lock — the same lock `gx tickets add` takes to allocate ids — and returns
// it alongside the loaded copy of path's own ticket. Only tickets living
// under the tracker's <epic>/issues/<file>.md layout have an epic to load: for
// anything else (ad-hoc files) every return value is nil, including the
// error, and the caller skips its epic-wide check. target is nil when the epic
// loaded but doesn't list path. The caller must call unlock (when non-nil),
// and must keep holding it for as long as it needs the loaded graph to stay
// authoritative.
func lockEpicForTicket(path string) (epic *tickets.Epic, target *tickets.Ticket, unlock func(), err error) {
	issuesDir := filepath.Dir(path)
	if filepath.Base(issuesDir) != "issues" {
		return nil, nil, nil, nil
	}

	epic, unlock, err = tickets.LoadLockedEpic(filepath.Dir(issuesDir))
	if err != nil {
		return nil, nil, nil, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		unlock()
		return nil, nil, nil, err
	}
	for i := range epic.Tickets {
		if tp, absErr := filepath.Abs(epic.Tickets[i].Path); absErr == nil && tp == absPath {
			target = &epic.Tickets[i]
			break
		}
	}
	return epic, target, unlock, nil
}

// lockEpicForParentWrite validates the parent graph path's epic would have
// once path's parent became parentID, and hands back the still-held epic lock
// so the caller can write under it. A rejected edge returns an error with the
// lock already released and nothing written.
func lockEpicForParentWrite(path, parentID string) (unlock func(), err error) {
	epic, target, unlock, err := lockEpicForTicket(path)
	if err != nil {
		return nil, fmt.Errorf("validating parent before writing: %w", err)
	}
	if target == nil {
		return unlock, nil
	}

	if parentID == "" {
		// Clearing a parent only ever removes an edge, so no graph it was
		// part of can become invalid.
		return unlock, nil
	}
	target.Parent = &parentID
	if err := epic.ValidateParentGraph(); err != nil {
		unlock()
		return nil, fmt.Errorf("%s: rejecting --parent %s: %w", path, parentID, err)
	}
	return unlock, nil
}

// checkIterationStatusFinishedGuard validates --iteration-status's value.
// Per no-silent-stalls Story 26/27, an agent's finished report can only start
// a merge, never conclude one — the zero-commit fault check belongs solely
// to ralphloop's own merge path, which has real git access to count
// commits (tickets 07/08), not to a CLI-visible proxy flag. Earlier this
// guard demanded --status=done alongside a non-commitless finished report,
// but checkAgentStatusGuard refuses that same --status write from a
// ralph-loop/* branch, leaving no call an iteration agent could legally make;
// see .scratch/conflict-lifecycle/issues/06-status-guard-contradiction-research.md.
func checkIterationStatusFinishedGuard(c *cobra.Command, path string) error {
	v, _ := c.Flags().GetString("iteration-status")
	if !schema.IterationStatus(v).Valid() {
		return fmt.Errorf("%s: --iteration-status %s is invalid; want working, asking, or finished", path, v)
	}
	return nil
}

// checkOpenQuestionGuard keeps a self-reported pause meaning "the latest
// question is open": moving a paused ticket to open, or reporting
// --iteration-status asking, both need a question in the log, and
// the refusal names the verb that writes one (ask) or ends one (answer).
func checkOpenQuestionGuard(c *cobra.Command, path string) error {
	var verb string
	if v, _ := c.Flags().GetString("iteration-status"); c.Flags().Changed("iteration-status") &&
		schema.IterationStatus(v) == schema.IterationStatusAsking {
		verb = "ask"
	}
	if v, _ := c.Flags().GetString("status"); c.Flags().Changed("status") && schema.Status(v) == schema.StatusOpen {
		current, err := schema.ParseTicket(path)
		if err != nil {
			return err
		}
		if current.PausedClass() == schema.PauseClassAnswer {
			verb = "answer"
		}
	}
	if verb == "" {
		return nil
	}
	evs, err := tickets.ReadLog(path)
	if err != nil {
		return err
	}
	if tickets.OpenQuestion(evs) != nil {
		return nil
	}
	return fmt.Errorf("%s: the log has no open question; use `gx tickets %s` instead of `tickets set`", ticketLabel(path), verb)
}

// isRalphLoopBranch is the shared branch-detection logic behind
// checkAgentStatusGuard.
func isRalphLoopBranch(getwd func() (string, error)) (branch string, ok bool) {
	return repair.IsRalphLoopBranch(getwd)
}

// checkBodyBeforeOpen refuses a --status open write for a ticket whose body
// is empty — the invariant "nothing schedulable is empty" is guarded here,
// at the draft-to-open transition, rather than at rest: schema.Validate (and
// so `gx tickets validate`) must keep accepting a body-less draft, since
// `gx tickets add` writes draft and then validates its own stub before
// returning.
func checkBodyBeforeOpen(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	body := schema.ParseBody(string(raw))
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("%s has an empty body; refusing to set status=open on a ticket with no content", path)
	}
	return nil
}
