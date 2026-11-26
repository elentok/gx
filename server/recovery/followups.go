package recovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketmove"
)

// DefaultFollowUps is the epic that receives follow-up drafts when
// recovery.follow-ups is unset.
const DefaultFollowUps = "gx:follow-ups"

// Proposal classes an investigate ticket's ## Result names. Only the first two
// are filed: human-only is already a person's job.
const (
	proposalCatalogEntry    = "catalog-entry"
	proposalOrchestratorFix = "orchestrator-fix"
	proposalHumanOnly       = "human-only"
)

var proposalClasses = []string{proposalCatalogEntry, proposalOrchestratorFix, proposalHumanOnly}

// proposalClass returns the one proposal class a ## Result names after its
// Proposal label. Zero or several distinct classes return "": a malformed
// report files nothing.
func proposalClass(result string) string {
	lower := strings.ToLower(result)
	at := strings.Index(lower, "proposal")
	if at < 0 {
		return ""
	}
	found := ""
	for _, c := range proposalClasses {
		if strings.Contains(lower[at:], c) {
			if found != "" {
				return ""
			}
			found = c
		}
	}
	return found
}

// FileFollowUp files a draft research ticket in the follow-ups epic for a merged
// investigate ticket whose report proposes a catalog entry or an orchestrator fix.
func (rec *Recovery) FileFollowUp(addr tickets.Address, ticketPath string) {
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.Type != schema.TypeInvestigate {
		return
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		rec.d.Log.Warn("read investigate report", "ticket", addr.String(), "err", err)
		return
	}
	result := strings.TrimSpace(schema.Section(schema.ParseBody(string(raw)), "Result"))
	class := proposalClass(result)
	if class != proposalCatalogEntry && class != proposalOrchestratorFix {
		return
	}
	key := dedupeKey(string(raw), class)
	path, err := rec.WriteFollowUp(addr, investigateFollowUp(addr, class, key, result))
	if errors.Is(err, errNoFollowUpProject) {
		// The proposal must not be lost with its filing place: the escalation
		// message carries it instead (it already does when the submit asked for
		// the result to be sent).
		rec.d.Log.Warn("follow-ups project missing, proposal goes to the escalation only", "ticket", addr.String(), "err", err)
		if !tk.Notify {
			dir := filepath.Dir(filepath.Dir(filepath.Dir(ticketPath)))
			rec.d.Chat.Result(addr.Project, rec.d.Projects.ChatOverride(addr.Project), dir, addr.Epic, ticketPath, addr.ID, result)
		}
		return
	}
	if err != nil {
		rec.d.Log.Warn("file follow-up", "ticket", addr.String(), "err", err)
		return
	}
	rec.d.Log.Info("filed follow-up", "ticket", addr.String(), "path", path)
}

var errNoFollowUpProject = errors.New("no follow-ups project")

// failureLine matches the line writeInvestigateTicket puts in an investigate
// ticket: the type and kind of the failure it was forked for.
var failureLine = regexp.MustCompile(`paused with (\S+) \(([^)]*)\)`)

// dedupeKey is the (type, kind, proposal class) that makes two follow-ups the
// same proposal. A report with no failure line still dedupes on its class.
func dedupeKey(investigateRaw, class string) string {
	m := failureLine.FindStringSubmatch(investigateRaw)
	if m == nil {
		return class
	}
	return m[1] + "/" + m[2] + "/" + class
}

// FollowUpsEpicPath is the follow-ups epic dir that receives follow-ups from
// fromProject's tickets.
func (rec *Recovery) FollowUpsEpicPath(fromProject string) (string, error) {
	target := rec.d.Settings.Load().RecoverySettings.FollowUps
	if target == "" {
		target = DefaultFollowUps
	}
	project, epic, ok := strings.Cut(target, ":")
	if !ok {
		project, epic = fromProject, target
	}
	dir, err := rec.d.Projects.Dir(project)
	if err != nil {
		return "", fmt.Errorf("%w %s: %v", errNoFollowUpProject, project, err)
	}
	return filepath.Join(dir, epic), nil
}

// investigateFollowUp is the follow-up for an investigate ticket's proposal.
func investigateFollowUp(from tickets.Address, class, key, result string) ticketwrite.FollowUp {
	return ticketwrite.FollowUp{
		Slug:         class,
		Title:        fmt.Sprintf("Follow up on %s (%s)", from, class),
		Source:       fmt.Sprintf("Investigate ticket %s, which proposed a %s.", from, class),
		Problem:      result,
		SuggestedFix: fmt.Sprintf("Assess the %s proposal in the Problem section; build it or drop it.", class),
		Key:          key,
	}
}

// WriteFollowUp files the follow-up, or counts a repeat on the open draft that
// already has the same key. from is the ticket the repeat is seen in.
func (rec *Recovery) WriteFollowUp(from tickets.Address, f ticketwrite.FollowUp) (string, error) {
	epicPath, err := rec.FollowUpsEpicPath(from.Project)
	if err != nil {
		return "", err
	}
	return WriteFollowUpIn(epicPath, from.String(), f)
}

// WriteFollowUpIn is WriteFollowUp into the epic at epicPath, for a repeat seen
// in source, a description of the place.
func WriteFollowUpIn(epicPath, source string, f ticketwrite.FollowUp) (string, error) {
	epic, unlock, err := lockDraftEpic(epicPath)
	if err != nil {
		return "", err
	}
	defer unlock()
	path, found, err := noteOccurrence(epicPath, f.Key, func(body string) string {
		return countRepeat(body, source, time.Now().Format("2006-01-02"))
	})
	if found || err != nil {
		return path, err
	}
	path, _, err = ticketwrite.WriteNewTicketLocked(*epic, "", f.Slug, func(id string) (schema.Ticket, string) {
		body := fmt.Sprintf(
			// The dedupe line sits in Evidence, the section a repeat appends to.
			"\n# %s — %s\n\n## Source\n\n%s\n\n## Problem\n\n%s\n\n## Evidence\n\n%s\n\n## Suggested fix\n\n%s\n",
			id, oneLine(f.Title), f.Source, f.Problem, dedupeLine(f.Key), f.SuggestedFix,
		)
		return schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusDraft, Type: schema.TypeResearch, Category: schema.CategoryBug}, body
	})
	return path, err
}

// lockDraftEpic creates a missing epic and locks it, so that looking for an open
// draft and writing it or a new one are one step.
func lockDraftEpic(epicPath string) (*tickets.Epic, func(), error) {
	if err := EnsureDraftEpic(epicPath); err != nil {
		return nil, nil, err
	}
	return ticketmove.LoadLockedEpic(epicPath)
}

// noteOccurrence rewrites with note the body of the open draft carrying key.
// Only a draft takes one: once a person opens the ticket a repeat is new work.
// The caller holds the epic lock.
func noteOccurrence(epicPath, key string, note func(body string) string) (path string, found bool, err error) {
	files, err := filepath.Glob(filepath.Join(epicPath, "issues", "*.md"))
	if err != nil {
		return "", false, err
	}
	for _, f := range files {
		tk, err := schema.ParseTicket(f)
		if err != nil || tk.Status != schema.StatusDraft || tk.Type != schema.TypeResearch {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil || !hasDedupeLine(schema.ParseBody(string(raw)), key) {
			continue
		}
		err = schema.UpdateTicketWithBody(f, func(_ *schema.Ticket, b *string) {
			*b = note(*b)
		})
		return f, true, err
	}
	return "", false, nil
}

// EnsureDraftEpic creates a missing epic with status draft, so nothing in it is
// scheduled until a person promotes it. An existing epic directory without a
// ticket.md gets one too; one with a ticket.md keeps it.
func EnsureDraftEpic(epicPath string) error {
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0o755); err != nil {
		return err
	}
	return tickets.WriteEpicTicketMD(epicPath, schema.StatusDraft)
}
