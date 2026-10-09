package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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

// fileFollowUp files a draft research ticket in the follow-ups epic for a landed
// investigate ticket whose report proposes a catalog entry or an orchestrator fix.
func (s *Server) fileFollowUp(addr tickets.Address, ticketPath string) {
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.Type != schema.TypeInvestigate {
		return
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		s.log.Warn("read investigate report", "ticket", addr.String(), "err", err)
		return
	}
	result := strings.TrimSpace(schema.Section(schema.ParseBody(string(raw)), "Result"))
	class := proposalClass(result)
	if class != proposalCatalogEntry && class != proposalOrchestratorFix {
		return
	}
	key := dedupeKey(string(raw), class)
	path, err := s.writeFollowUp(addr, class, key, result)
	if errors.Is(err, errNoFollowUpProject) {
		// The proposal must not be lost with its filing place: the escalation
		// message carries it instead (it already does when the submit asked for
		// the result to be sent).
		s.log.Warn("follow-ups project missing, proposal goes to the escalation only", "ticket", addr.String(), "err", err)
		if !tk.Notify {
			dir := filepath.Dir(filepath.Dir(filepath.Dir(ticketPath)))
			s.chat.Result(addr.Project, s.chatOverride(addr.Project), dir, addr.Epic, addr.String(), result)
		}
		return
	}
	if err != nil {
		s.log.Warn("file follow-up", "ticket", addr.String(), "err", err)
		return
	}
	s.log.Info("filed follow-up", "ticket", addr.String(), "path", path)
}

var errNoFollowUpProject = errors.New("no follow-ups project")

// failureLine matches the line writeInvestigateTicket puts in an investigate
// ticket: the type and kind of the failure it was forked for.
var failureLine = regexp.MustCompile(`parked with (\S+) \(([^)]*)\)`)

// dedupeKey is the (type, kind, proposal class) that makes two follow-ups the
// same proposal. A report with no failure line still dedupes on its class.
func dedupeKey(investigateRaw, class string) string {
	m := failureLine.FindStringSubmatch(investigateRaw)
	if m == nil {
		return class
	}
	return m[1] + "/" + m[2] + "/" + class
}

func dedupeLine(key string) string { return "Dedupe key: " + key }

// writeFollowUp files the follow-up, or adds an occurrence line to the open
// draft that already has the same key.
func (s *Server) writeFollowUp(from tickets.Address, class, key, result string) (string, error) {
	target := s.cfg.RecoverySettings.FollowUps
	if target == "" {
		target = DefaultFollowUps
	}
	project, epic, ok := strings.Cut(target, ":")
	if !ok {
		project, epic = from.Project, target
	}
	dir, err := s.projectDir(project)
	if err != nil {
		return "", fmt.Errorf("%w %s: %v", errNoFollowUpProject, project, err)
	}
	epicPath := filepath.Join(dir, epic)
	if err := ensureDraftEpic(epicPath); err != nil {
		return "", err
	}
	if path, found, err := noteOccurrence(epicPath, key, from); found || err != nil {
		return path, err
	}
	path, _, err := writeNewTicket(epicPath, "", class, func(id string) (schema.Ticket, string) {
		body := fmt.Sprintf(
			"\n# %s — Follow up on %s (%s)\n\n## What to build\n\n%s\n\nInvestigate %s proposed a %s. Its report:\n\n%s\n\n## Acceptance criteria\n\n- [ ] Proposal assessed and either built or dropped\n",
			id, from, class, dedupeLine(key), from, class, result,
		)
		return schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusDraft, Type: schema.TypeResearch}, body
	})
	return path, err
}

// noteOccurrence appends an occurrence line to the open draft carrying key.
// Only a draft takes one: once a person opens the ticket a repeat is new work.
func noteOccurrence(epicPath, key string, from tickets.Address) (path string, found bool, err error) {
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
		if err != nil || !strings.Contains(string(raw), dedupeLine(key)+"\n") {
			continue
		}
		err = schema.UpdateTicketWithBody(f, func(_ *schema.Ticket, b *string) {
			*b = schema.AppendComment(*b, fmt.Sprintf("Seen again in %s on %s.", from, time.Now().Format("2006-01-02")))
		})
		return f, true, err
	}
	return "", false, nil
}

// ensureDraftEpic creates a missing epic with status draft, so nothing in it is
// scheduled until a person promotes it. An existing epic directory without a
// ticket.md gets one too; one with a ticket.md keeps it.
func ensureDraftEpic(epicPath string) error {
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0o755); err != nil {
		return err
	}
	return tickets.WriteEpicTicketMD(epicPath, schema.StatusDraft)
}
