package ticketsflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/ticketref"
	ticketscmd "github.com/elentok/gx/cmd/tickets"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketmove"
	"github.com/elentok/gx/tickets/ticketspec"
	"github.com/spf13/cobra"
)

// publishedTicket is one row of publish's output.
type publishedTicket struct {
	Slug    string `json:"slug"`
	Address string `json:"address"`
	Window  int    `json:"window"`
}

// plannedTicket is a manifest entry rendered in memory: its ID, path and the
// bytes of both the draft file written first and the open file written last.
type plannedTicket struct {
	publishedTicket
	ID    string
	Path  string
	Type  string
	Body  string
	Draft []byte
	Open  []byte
}

type publishPlan struct {
	EpicPath         string
	Tickets          []plannedTicket
	AllowCheckMisses bool
	// Promoted maps each follow-up address the manifest names to the addresses
	// of the new tickets that cite it, in the order the follow-ups first appear.
	Promoted      map[string][]string
	PromotedOrder []string
}

// resolveFrom turns a manifest `from:` address into its canonical form, or
// refuses it when it is not a draft/open follow-up of the target's project.
type resolveFrom func(addr string) (string, error)

// followUpResolver is the resolveFrom of project: every address must name a
// draft or open ticket in that project's follow-ups epic.
func followUpResolver(d Deps, cwd, project string) resolveFrom {
	return func(addr string) (string, error) {
		a, t, err := ticketref.FindTicket(d.dirs(), cwd, addr)
		if err != nil {
			return "", fmt.Errorf("from %q: %w", addr, err)
		}
		switch {
		case a.Project != project:
			return "", fmt.Errorf("from %q: follow-up is in project %q, not %q", addr, a.Project, project)
		case a.Epic != tickets.FollowUpsEpic:
			return "", fmt.Errorf("from %q: not a ticket of a %s epic", addr, tickets.FollowUpsEpic)
		case t.Status != string(schema.StatusDraft) && t.Status != string(schema.StatusOpen):
			return "", fmt.Errorf("from %q: follow-up is %s; want draft or open", addr, t.Status)
		}
		return a.String(), nil
	}
}

func (p *publishPlan) paths() []string {
	out := make([]string, len(p.Tickets))
	for i, t := range p.Tickets {
		out[i] = t.Path
	}
	return out
}

func (p *publishPlan) rows() []publishedTicket {
	out := make([]publishedTicket, len(p.Tickets))
	for i, t := range p.Tickets {
		out[i] = t.publishedTicket
	}
	return out
}

// publishPreWriteChecks run in order over the rendered plan before the first
// write; each returns the failures it found. Later checks (lint, advisories)
// are added here.
var publishPreWriteChecks = []func(*publishPlan) []error{
	checkRenderedTickets,
	checkDraftMarkers,
	checkPublishShape,
}

// checkDraftMarkers refuses a body with an unwritten `draft` part. Unlike the
// shape check, --allow-check-misses does not skip it.
func checkDraftMarkers(p *publishPlan) []error {
	var errs []error
	for _, t := range p.Tickets {
		if ticketspec.HasDraftMarker(t.Body) {
			errs = append(errs, fmt.Errorf("%s: body still has a %s marker; write What to build and Acceptance criteria", t.Slug, ticketspec.DraftMarker))
		}
	}
	return errs
}

// checkPublishShape refuses implement tickets missing a required section or over
// the split limit, unless --allow-check-misses. It has no bearing on claim lint.
func checkPublishShape(p *publishPlan) []error {
	if p.AllowCheckMisses {
		return nil
	}
	var errs []error
	for _, t := range p.Tickets {
		for _, miss := range ticketspec.PublishShapeMisses(t.Type, t.Body, t.Window) {
			errs = append(errs, fmt.Errorf("%s: %s (--allow-check-misses publishes anyway)", t.Slug, miss))
		}
	}
	return errs
}

// checkRenderedTickets runs the ticket validation of `gx tickets validate` over
// each rendered ticket, in memory.
func checkRenderedTickets(p *publishPlan) []error {
	var errs []error
	for _, t := range p.Tickets {
		if _, err := schema.ParseTicketFromRaw(string(t.Open), t.Path); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// planPublish reads and checks the manifest against the epic's current tickets
// and renders every ticket, assigning IDs in manifest order and stamping
// {{commit}} with commit. Nothing is written.
func planPublish(epicPath, manifestPath, commit string, allowCheckMisses bool, epic tickets.Epic, resolve resolveFrom) (*publishPlan, error) {
	var epicIDs []string
	for _, t := range epic.Tickets {
		epicIDs = append(epicIDs, t.Identifier)
	}
	m, err := ticketspec.ParseManifest(manifestPath, epicIDs)
	if err != nil {
		return nil, err
	}
	first, err := ticketmove.NextTicketID(epic, "")
	if err != nil {
		return nil, err
	}
	start, err := strconv.Atoi(first)
	if err != nil {
		return nil, err
	}
	ids := map[string]string{}
	for i, e := range m.Entries {
		ids[e.Slug] = fmt.Sprintf("%02d", start+i)
	}

	// The renderer's one pass also stamps {{commit}}; the slug "commit" is reserved.
	bodyIDs := map[string]string{ticketspec.CommitSlug: commit}
	for slug, id := range ids {
		bodyIDs[slug] = id
	}

	plan := &publishPlan{EpicPath: epicPath, AllowCheckMisses: allowCheckMisses, Promoted: map[string][]string{}}
	var fromErrs []error
	now := time.Now().UTC()
	for _, e := range m.Entries {
		id := ids[e.Slug]
		path := filepath.Join(epicPath, "issues", fmt.Sprintf("%s-%s.md", id, e.Slug))
		address, ok := tickets.AddressOfPath(path, id)
		if !ok {
			return nil, fmt.Errorf("no address for new ticket %s", path)
		}
		t := schema.Ticket{
			ID:                    schema.TicketID(id),
			Status:                schema.StatusDraft,
			Type:                  schema.TicketType(e.Type),
			ExpectedContextWindow: e.ExpectedContextWindow,
		}
		for _, ref := range e.BlockedBy {
			if mapped, ok := ids[ref]; ok {
				ref = mapped
			}
			t.BlockedBy = append(t.BlockedBy, schema.TicketID(ref))
		}
		origins := []string{address.String()}
		if len(e.From) > 0 {
			origins = origins[:0]
			for _, raw := range e.From {
				canon, err := resolve(raw)
				if err != nil {
					fromErrs = append(fromErrs, fmt.Errorf("%s: %w", e.Slug, err))
					continue
				}
				if _, seen := plan.Promoted[canon]; !seen {
					plan.PromotedOrder = append(plan.PromotedOrder, canon)
				}
				plan.Promoted[canon] = append(plan.Promoted[canon], address.String())
				origins = append(origins, canon)
			}
		}
		t.StampCreated(now, origins...)
		rendered := ticketspec.RenderManifestBody(e.Body, bodyIDs)
		text := "\n" + strings.Trim(rendered, "\n") + "\n"
		draft, err := schema.MarshalTicket(t, text)
		if err != nil {
			return nil, fmt.Errorf("marshaling %s: %w", e.Slug, err)
		}
		t.Status = schema.StatusOpen
		open, err := schema.MarshalTicket(t, text)
		if err != nil {
			return nil, fmt.Errorf("marshaling %s: %w", e.Slug, err)
		}
		plan.Tickets = append(plan.Tickets, plannedTicket{
			publishedTicket: publishedTicket{Slug: e.Slug, Address: address.String(), Window: e.ExpectedContextWindow},
			ID:              id,
			Path:            path,
			Type:            e.Type,
			Body:            rendered,
			Draft:           draft,
			Open:            open,
		})
	}

	if err := errors.Join(fromErrs...); err != nil {
		return nil, err
	}
	var errs []error
	for _, check := range publishPreWriteChecks {
		errs = append(errs, check(plan)...)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return plan, nil
}

// printBundledAdvisories prints one line per implement ticket that reaches both
// bundled-ticket thresholds. It is advice only and never refuses.
func printBundledAdvisories(w io.Writer, cwd string, plan *publishPlan) {
	root, err := git.Toplevel(cwd)
	if err != nil {
		return
	}
	topDirs := topLevelDirs(root)
	for _, t := range plan.Tickets {
		if line := ticketspec.BundledAdvisory(t.Slug, t.Type, t.Body, topDirs); line != "" {
			fmt.Fprintln(w, line)
		}
	}
}

// fileWriter is Deps.WriteFile, or os.WriteFile.
func (d Deps) fileWriter() func(string, []byte, os.FileMode) error {
	if d.WriteFile != nil {
		return d.WriteFile
	}
	return os.WriteFile
}

// runTicketsPublish publishes the manifest's tickets into the epic as one unit:
// every ticket is written as draft, checked, then all are opened in one pass at
// the end. Any failure removes what this run wrote (and the epic, if it created
// it), so a retry assigns the same IDs.
func runTicketsPublish(d Deps, epicArg, manifestPath string, dryRun, jsonOut, allowClaimMisses, allowCheckMisses bool, w io.Writer) error {
	plan, err := d.publishTickets(epicArg, manifestPath, dryRun, jsonOut, allowClaimMisses, allowCheckMisses, w)
	if err != nil || plan == nil {
		return err
	}
	// Follow-ups are archived after publishTickets returns, so its rollback and
	// epic lock are gone: a failure here leaves the tickets in place. The archive
	// only touches the follow-ups epic, so running it unlocked is safe.
	return d.archivePromoted(plan)
}

// publishTickets does the whole publish up to printing the table. It returns a
// nil plan for a dry run, which writes nothing. The plan is a local, not a named
// result: a bare `return nil, err` would clear it before the rollback defers run.
func (d Deps) publishTickets(epicArg, manifestPath string, dryRun, jsonOut, allowClaimMisses, allowCheckMisses bool, w io.Writer) (_ *publishPlan, err error) {
	cwd, err := d.Getwd()
	if err != nil {
		return nil, err
	}
	// One commit per run: it stamps {{commit}} and heads the lint report.
	commit, err := shortHeadAt(cwd)
	if err != nil {
		return nil, err
	}
	projectDir, project, name, err := resolveEpicTarget(d.dirs(), cwd, epicArg)
	if err != nil {
		return nil, err
	}
	epicPath := filepath.Join(projectDir, name)
	resolve := followUpResolver(d, cwd, project)

	info, statErr := os.Stat(epicPath)
	epicExists := statErr == nil && info.IsDir()

	var plan *publishPlan
	var unlock func()
	if epicExists {
		var epic *tickets.Epic
		if epic, unlock, err = ticketmove.LoadLockedEpic(epicPath); err != nil {
			return nil, err
		}
		defer func() { unlock() }()
		if plan, err = planPublish(epicPath, manifestPath, commit, allowCheckMisses, *epic, resolve); err != nil {
			return nil, err
		}
	} else {
		if err := refuseTakenEpicName(projectDir, project, name); err != nil {
			return nil, err
		}
		if plan, err = planPublish(epicPath, manifestPath, commit, allowCheckMisses, tickets.Epic{}, resolve); err != nil {
			return nil, err
		}
	}

	// The lint report goes to stderr when the table is JSON, so stdout stays parseable.
	lintOut := w
	if jsonOut && d.Stderr != nil {
		lintOut = d.Stderr
	}
	report, err := lintEpicClaims(d, cwd, commit, epicPath, manifestPath, lintOut)
	if err != nil {
		return nil, err
	}
	if report.Refused() && !allowClaimMisses {
		return nil, &cmdbase.ExitError{Code: 1}
	}
	printBundledAdvisories(lintOut, cwd, plan)

	if dryRun {
		return nil, printPublished(w, plan.rows(), jsonOut)
	}

	if !epicExists {
		if err := runTicketsEpicAdd(d.dirs(), cwd, epicArg, tickets.KindLoop, string(schema.StatusDraft), "", io.Discard); err != nil {
			return nil, err
		}
		// Creating the epic may stamp a map's completion; that stamp stays on rollback.
		defer func() {
			if err != nil {
				_ = os.RemoveAll(epicPath)
			}
		}()
		var epic *tickets.Epic
		if epic, unlock, err = ticketmove.LoadLockedEpic(epicPath); err != nil {
			return nil, err
		}
		defer func() { unlock() }()
		if plan, err = planPublish(epicPath, manifestPath, commit, allowCheckMisses, *epic, resolve); err != nil {
			return nil, err
		}
	}

	write := d.fileWriter()
	defer func() {
		if err != nil {
			for _, t := range plan.Tickets {
				_ = os.Remove(t.Path)
			}
		}
	}()
	for _, t := range plan.Tickets {
		if err = write(t.Path, t.Draft, 0o644); err != nil {
			return nil, fmt.Errorf("writing ticket %s: %w", t.Path, err)
		}
	}
	if err = validateNewTickets(projectDir, plan.paths()); err != nil {
		return nil, err
	}
	for _, t := range plan.Tickets {
		if err = write(t.Path, t.Open, 0o644); err != nil {
			return nil, fmt.Errorf("opening ticket %s: %w", t.Path, err)
		}
	}
	for _, t := range plan.Tickets {
		ticketscmd.PingServer(d.Env, t.Path)
	}
	if err = printPublished(w, plan.rows(), jsonOut); err != nil {
		return nil, err
	}
	return plan, nil
}

// archivePromoted closes each follow-up the plan promoted as `promoted`, noting
// the new addresses. It stops at the first failure and names the follow-up.
func (d Deps) archivePromoted(plan *publishPlan) error {
	closeFn := d.CloseFollowUp
	if closeFn == nil {
		closeFn = func(addr, reason, note string, w io.Writer) error {
			return ticketscmd.CloseFollowUp(d.Env, addr, reason, note, w)
		}
	}
	for _, from := range plan.PromotedOrder {
		note := "promoted to " + strings.Join(plan.Promoted[from], ", ")
		if err := closeFn(from, ticketscmd.ReasonPromoted, note, io.Discard); err != nil {
			return fmt.Errorf("tickets published, but archiving follow-up %s failed: %w", from, err)
		}
	}
	return nil
}

// validateNewTickets is tickets.ValidateProject limited to the failures that
// name one of paths.
func validateNewTickets(projectDir string, paths []string) error {
	err := tickets.ValidateProject(projectDir)
	if err == nil {
		return nil
	}
	var all []error
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		all = j.Unwrap()
	} else {
		all = []error{err}
	}
	var mine []error
	for _, e := range all {
		for _, p := range paths {
			if strings.Contains(e.Error(), p) {
				mine = append(mine, e)
				break
			}
		}
	}
	return errors.Join(mine...)
}

func printPublished(w io.Writer, rows []publishedTicket, jsonOut bool) error {
	if jsonOut {
		return json.NewEncoder(w).Encode(rows)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SLUG\tADDRESS\tWINDOW")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%d\n", r.Slug, r.Address, r.Window)
	}
	return tw.Flush()
}

func newTicketsPublishCmd(d Deps) *cobra.Command {
	var manifest string
	var dryRun, jsonOut, allowClaimMisses, allowCheckMisses bool
	cmd := &cobra.Command{
		Use:   "publish <project:epic>",
		Short: "write a manifest's tickets into an epic as draft, then open them all at once",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runTicketsPublish(d, args[0], manifest, dryRun, jsonOut, allowClaimMisses, allowCheckMisses, c.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&manifest, "manifest", "", "manifest file (YAML list of slug, type, blocked_by, expected_context_window, body)")
	_ = cmd.MarkFlagRequired("manifest")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the IDs that would be assigned and write nothing")
	cmd.Flags().BoolVar(&allowClaimMisses, "allow-claim-misses", false, "publish even when the claim lint finds a miss in an implement or research ticket")
	cmd.Flags().BoolVar(&allowCheckMisses, "allow-check-misses", false, "publish even when an implement ticket lacks ## Test seams or ## Where to look, or its window is over the split limit")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit [{slug, address, window}] JSON instead of a table")
	return cmd
}
