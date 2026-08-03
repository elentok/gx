package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/tickets/schema"
)

// Claim writes status: claimed into the ticket file at path.
func Claim(path string) error {
	return SetStatus(path, "claimed")
}

// MarkDone writes status: done into the ticket file at path.
func MarkDone(path string) error {
	return SetStatus(path, "done")
}

// MarkNeedsInfo writes status: needs-info into the ticket file at path.
func MarkNeedsInfo(path string) error {
	return SetStatus(path, "needs-info")
}

// MarkNeedsAttentionWithReason writes status: needs-attention into the
// ticket file and appends reason to the ticket's body under a "## Needs
// Attention" heading, so the full failure is readable by opening the ticket
// file even when the live UI's status subtext truncates it.
func MarkNeedsAttentionWithReason(path, reason string) error {
	return updateTicketWithBody(path, func(t *schema.Ticket, body *string) {
		t.Status = schema.StatusNeedsAttention
		*body += fmt.Sprintf("\n## Needs Attention\n\n%s\n", reason)
	})
}

// MarkDoneWithMetadata marks a ticket done and records the closing
// iteration's final context-window occupancy and compaction count in the
// frontmatter's actual_context_window/compactions fields. sessionID is
// accepted for caller compatibility but no longer persisted: per
// .scratch/ticket-frontmatter/spec.md, the Session field is dropped from
// frontmatter entirely (not validated or read by code). status,
// actual_context_window, and compactions land in a single atomic write, so a
// concurrent reader never observes one without the others.
func MarkDoneWithMetadata(path string, contextWindow, compactions int, sessionID string) error {
	return updateTicket(path, func(t *schema.Ticket) {
		t.Status = schema.StatusDone
		t.ActualContextWindow = contextWindow
		t.Compactions = compactions
	})
}

// SetStatus rewrites (or, if the ticket's status is unset, sets for the
// first time) a ticket file's frontmatter status field to value, leaving
// the rest of the file - other frontmatter fields and the body - unchanged.
func SetStatus(path, value string) error {
	return updateTicket(path, func(t *schema.Ticket) {
		t.Status = schema.Status(value)
	})
}

// updateTicket reads path's frontmatter ticket, applies mutate to it, and
// writes the result back via schema.MarshalTicket, leaving the body
// untouched. The write goes through writeFileAtomic so a concurrent reader
// (the scheduler re-scanning the frontier while another goroutine
// claims/completes a ticket) always sees either the old or the new content
// in full, never a torn/truncated write from an in-place os.WriteFile.
func updateTicket(path string, mutate func(*schema.Ticket)) error {
	return updateTicketWithBody(path, func(t *schema.Ticket, _ *string) {
		mutate(t)
	})
}

// updateTicketWithBody is updateTicket's variant for mutations that also
// need to rewrite the ticket's markdown body (e.g. appending a note),
// leaving the same atomic-write/torn-read guarantees.
func updateTicketWithBody(path string, mutate func(*schema.Ticket, *string)) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	t, err := schema.ParseTicketFromRaw(string(raw), path)
	if err != nil {
		return fmt.Errorf("reading ticket %s: %w", path, err)
	}
	body := schema.ParseBody(string(raw))

	mutate(&t, &body)

	out, err := schema.MarshalTicket(t, body)
	if err != nil {
		return fmt.Errorf("marshaling ticket %s: %w", path, err)
	}
	return writeFileAtomic(path, out)
}

// writeFileAtomic replaces path's content via a same-directory temp file
// plus rename, so a concurrent reader (the scheduler re-scanning the
// frontier while another goroutine claims/completes a ticket) always sees
// either the old or the new content in full, never a torn/truncated write
// from an in-place os.WriteFile.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
