package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// MigrationChange reports what changed for one ticket file during Migrate,
// as a human-readable note per changed field.
type MigrationChange struct {
	Path  string
	Notes []string
}

// MigrationResult is Migrate's report, in Path-sorted order. A nil Changes
// means scratchRoot was already fully in the post-refactor shape.
type MigrationResult struct {
	Changes []MigrationChange
}

// migratedTicketFile is one on-disk ticket file's before/after state, tracked
// while Migrate walks scratchRoot so nothing is written until every file has
// been computed and validated.
type migratedTicketFile struct {
	path      string
	body      string
	newTicket schema.Ticket
	notes     []string
}

// Migrate rewrites every ticket under scratchRoot into the post-refactor
// frontmatter shape: it drops the retired `children` field (trusting each
// ticket's own `parent` edge instead — see migrateTicket), drops a
// `blocked_by` entry naming the ticket's own parent, stamps an explicit
// `status` on any ticket that omits one, and maps the three retiring status
// values onto their replacements.
//
// The whole tracker root is one transaction: every ticket's migrated result
// is computed and validated — including, via Epic.ValidateParentGraph, every
// epic's parent graph — before anything is written. If any part of the
// result would be invalid, nothing is written and the returned error names
// what failed. A ticket whose migration is a no-op is left byte-identical: a
// second run over an already-migrated tree reports no changes and writes
// nothing.
func Migrate(scratchRoot string) (MigrationResult, error) {
	entries, err := os.ReadDir(scratchRoot)
	if err != nil {
		return MigrationResult{}, fmt.Errorf("reading tracker root %s: %w", scratchRoot, err)
	}

	var epics []Epic
	var files []migratedTicketFile

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		epicPath := filepath.Join(scratchRoot, entry.Name())
		issuesDir := filepath.Join(epicPath, "issues")
		issueEntries, err := os.ReadDir(issuesDir)
		if err != nil {
			continue
		}

		epic := Epic{Name: entry.Name(), Path: epicPath}

		for _, issueEntry := range issueEntries {
			if issueEntry.IsDir() {
				continue
			}
			number, identifier, title, ok := parseTicketFilename(issueEntry.Name())
			if !ok {
				continue
			}

			ticketPath := filepath.Join(issuesDir, issueEntry.Name())
			raw, err := os.ReadFile(ticketPath)
			if err != nil {
				return MigrationResult{}, fmt.Errorf("reading %s: %w", ticketPath, err)
			}

			old, err := schema.ParseTicketRaw(string(raw), ticketPath)
			if err != nil {
				return MigrationResult{}, fmt.Errorf("parsing %s: %w", ticketPath, err)
			}
			newTicket, notes := migrateTicket(old)

			files = append(files, migratedTicketFile{
				path:      ticketPath,
				body:      schema.ParseBody(string(raw)),
				newTicket: newTicket,
				notes:     notes,
			})

			epic.Tickets = append(epic.Tickets, Ticket{
				Number:     number,
				Identifier: identifier,
				Title:      title,
				Path:       ticketPath,
				Type:       string(newTicket.Type),
				BlockedBy:  idsToStrings(newTicket.BlockedBy),
				Parent:     idToStringPtr(newTicket.Parent),
				Status:     string(newTicket.Status),
			})
		}

		epics = append(epics, epic)
	}

	if err := validateMigration(files, epics); err != nil {
		return MigrationResult{}, err
	}

	return writeMigration(files)
}

// validateMigration checks every file's migrated ticket via schema.Validate
// and every epic's migrated parent graph via Epic.ValidateParentGraph,
// joining every failure so a caller sees the whole picture at once rather
// than stopping at the first bad file.
func validateMigration(files []migratedTicketFile, epics []Epic) error {
	var errs []error
	for _, f := range files {
		if err := schema.Validate(f.newTicket); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.path, err))
		}
	}
	for _, epic := range epics {
		if err := epic.ValidateParentGraph(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", epic.Path, err))
		}
	}
	return errors.Join(errs...)
}

// writeMigration writes only the files that actually changed (non-empty
// notes) — a file migrateTicket left untouched keeps its exact original
// bytes rather than being rewritten to a byte-identical copy.
func writeMigration(files []migratedTicketFile) (MigrationResult, error) {
	var result MigrationResult
	for _, f := range files {
		if len(f.notes) == 0 {
			continue
		}
		out, err := schema.MarshalTicket(f.newTicket, f.body)
		if err != nil {
			return MigrationResult{}, fmt.Errorf("marshaling %s: %w", f.path, err)
		}
		if err := writeFileAtomic(f.path, out); err != nil {
			return MigrationResult{}, fmt.Errorf("writing %s: %w", f.path, err)
		}
		result.Changes = append(result.Changes, MigrationChange{Path: f.path, Notes: f.notes})
	}
	sort.Slice(result.Changes, func(i, j int) bool { return result.Changes[i].Path < result.Changes[j].Path })
	return result, nil
}

// The retired status values, named here rather than in schema: they are no
// longer members of the status enum, and migration is the only code left
// that has to recognize them at all.
const (
	legacyStatusNeedsTriage    schema.Status = "needs-triage"
	legacyStatusReadyForAgent  schema.Status = "ready-for-agent"
	legacyStatusReadyForHuman  schema.Status = "ready-for-human"
	legacyStatusNeedsInfo      schema.Status = "needs-info"
	legacyStatusNeedsAttention schema.Status = "needs-attention"
)

// migratedStatusOf maps the status values the lifecycle-refactor and
// no-silent-stalls contractions retire onto their replacements, and stamps
// "open" onto a ticket that omits status: entirely (today, a missing
// status: silently reads as open — see tickets.openStatuses — which is
// also a way around draft for a hand-authored empty file; requiring
// status: explicit is what makes the coming contraction safe). Every other
// status passes through unchanged.
func migratedStatusOf(s schema.Status) schema.Status {
	switch s {
	case "":
		return schema.StatusOpen
	case legacyStatusReadyForAgent:
		return schema.StatusOpen
	case legacyStatusNeedsTriage:
		return schema.StatusDraft
	case legacyStatusReadyForHuman:
		// "Handed back to a human" is exactly what needs-answer now covers;
		// mapping to open would let the orchestrator re-claim a ticket that
		// was deliberately handed back.
		return schema.StatusNeedsAnswer
	case legacyStatusNeedsInfo:
		return schema.StatusNeedsAnswer
	case legacyStatusNeedsAttention:
		return schema.StatusNeedsRepair
	default:
		return s
	}
}

// migrateTicket returns old rewritten into the post-refactor shape, plus a
// note per field it actually changed. A nil notes slice means old already
// matched the new shape, the signal writeMigration uses to leave the file
// untouched. It takes the raw parse rather than the typed ticket because the
// retired children field no longer has a home on schema.Ticket: a rewrite
// drops it on its own, and migration's only remaining job is to notice the
// field was there and report it.
//
// Dropping children unconditionally — rather than trying to reconcile it
// against Parent — is the fix for the malformed-fork-chain gotcha (a fork
// root listing both its direct fork and that fork's own child): Parent is
// the edge every other consumer (Epic.ValidateParentGraph,
// Epic.UnresolvedBlockers) already treats as authoritative, so discarding
// children never loses information a correct graph needs.
func migrateTicket(old schema.RawTicket) (schema.Ticket, []string) {
	out := old.Ticket
	var notes []string

	if old.HasChildren {
		notes = append(notes, "children removed")
	}

	if out.Parent != nil && len(out.BlockedBy) > 0 {
		var kept []schema.TicketID
		for _, b := range out.BlockedBy {
			if b == *out.Parent {
				notes = append(notes, fmt.Sprintf("blocked_by: removed self-parent entry %q", b))
				continue
			}
			kept = append(kept, b)
		}
		out.BlockedBy = kept
	}

	if migrated := migratedStatusOf(out.Status); migrated != out.Status {
		from := string(out.Status)
		if from == "" {
			from = "(missing)"
		}
		notes = append(notes, fmt.Sprintf("status: %s -> %s", from, migrated))
		out.Status = migrated
	}

	return out, notes
}
