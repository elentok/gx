package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

// Claim writes status: claimed into the ticket file at path, clearing any
// iteration_status left over from a prior claim of this ticket — a stale
// self-report must not survive into a fresh claim. It also demotes any
// "## Needs Repair" section into a dated "## Comments" sub-entry: per the
// retirement principle (only a person reads a fault section, so gx — not
// the next agent — retires it), and claim is the moment a ticket starts a
// new life. Doing this here rather than at reattach means a ticket that
// reattaches several times within one claim never re-fires the demotion.
func Claim(path string) error {
	return updateTicketWithBody(path, func(t *schema.Ticket, body *string) {
		t.Status = schema.StatusClaimed
		t.IterationStatus = ""
		*body = demoteNeedsRepairSection(*body, time.Now())
	})
}

// bodySection is one "## Heading" block of a ticket body: the heading line
// itself plus everything up to (not including) the next top-level heading.
type bodySection struct {
	heading string
	content string
}

// splitBodySections splits body into the free text before its first "## "
// heading (preamble) and the ordered list of heading blocks that follow.
// Rejoining preamble and sections with joinBodySections reproduces body
// exactly when neither is modified.
func splitBodySections(body string) (preamble string, sections []bodySection) {
	lines := strings.Split(body, "\n")
	first := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## ") {
			first = i
			break
		}
	}
	if first == -1 {
		return body, nil
	}
	preamble = strings.Join(lines[:first], "\n")
	i := first
	for i < len(lines) {
		heading := lines[i]
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(lines[j], "## ") {
			j++
		}
		sections = append(sections, bodySection{heading: heading, content: strings.Join(lines[i+1:j], "\n")})
		i = j
	}
	return preamble, sections
}

// joinBodySections is splitBodySections's inverse.
func joinBodySections(preamble string, sections []bodySection) string {
	parts := []string{}
	if preamble != "" {
		parts = append(parts, preamble)
	}
	for _, s := range sections {
		parts = append(parts, s.heading+"\n"+s.content)
	}
	return strings.Join(parts, "\n")
}

// demoteNeedsRepairSection moves body's "## Needs Repair" section (if any)
// into a dated sub-entry appended under "## Comments" — creating that
// heading if it doesn't exist yet — and removes the "## Needs Repair"
// heading itself. Replacing rather than appending a fresh "## Needs Repair"
// on top is what fixes the live stacking bug: a body with no such section is
// returned unchanged, so a second claim in a row is a no-op here.
func demoteNeedsRepairSection(body string, now time.Time) string {
	preamble, sections := splitBodySections(body)

	idx := -1
	for i, s := range sections {
		if s.heading == "## Needs Repair" {
			idx = i
			break
		}
	}
	if idx == -1 {
		return body
	}

	reason := strings.TrimSpace(sections[idx].content)
	entry := fmt.Sprintf("**%s** — retired from `## Needs Repair`:\n\n%s\n", now.Format("2006-01-02"), reason)
	sections = append(sections[:idx], sections[idx+1:]...)

	commentsIdx := -1
	for i, s := range sections {
		if s.heading == "## Comments" {
			commentsIdx = i
			break
		}
	}
	if commentsIdx == -1 {
		sections = append(sections, bodySection{heading: "## Comments", content: "\n" + entry})
	} else {
		existing := strings.TrimRight(sections[commentsIdx].content, "\n")
		if strings.TrimSpace(existing) == "" {
			sections[commentsIdx].content = "\n" + entry
		} else {
			sections[commentsIdx].content = existing + "\n\n" + entry
		}
	}

	return joinBodySections(preamble, sections)
}

// MarkDone writes status: done into the ticket file at path.
func MarkDone(path string) error {
	return SetStatus(path, "done")
}

// MarkNeedsAnswer writes status: needs-answer into the ticket file at path.
func MarkNeedsAnswer(path string) error {
	return SetStatus(path, "needs-answer")
}

// MarkNeedsAnswerWithReasonAndStub writes status: needs-answer into the
// ticket file and appends reason to the body under a "## Needs Answer"
// heading, both naming label. This is the pane-answered park (an involuntary
// prompt the agent didn't choose): the stub distinguishes it from
// MarkNeedsAnswer's bare write for a ticket-answered zero-commit finish,
// since the question here exists only in the pane — a person answers it
// there, and the stub just gives them (and the TUI's auto-scroll) something
// to find.
func MarkNeedsAnswerWithReasonAndStub(path, reason string) error {
	return updateTicketWithBody(path, func(t *schema.Ticket, body *string) {
		t.Status = schema.StatusNeedsAnswer
		*body += fmt.Sprintf("\n## Needs Answer\n\n%s\n", reason)
	})
}

// MarkNeedsRepairWithReason writes status: needs-repair into the ticket file
// and appends a "## Needs Repair" section built by schema.FormatNeedsRepairBody
// (summary/detail split from reason, plus state rendered best-effort), so the
// full failure is readable by opening the ticket file even when the live
// UI's status subtext truncates it. Fails without writing anything if reason
// is empty — see FormatNeedsRepairBody's write-conditional validation.
func MarkNeedsRepairWithReason(path, reason string, state schema.NeedsRepairState) error {
	section, err := schema.FormatNeedsRepairBody(reason, state)
	if err != nil {
		return err
	}
	return updateTicketWithBody(path, func(t *schema.Ticket, body *string) {
		t.Status = schema.StatusNeedsRepair
		*body += section
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

// AppendSessionID appends sessionID to the ticket's session_ids frontmatter
// list, leaving every existing entry untouched — every fresh launch or
// reattach gets its own entry so a ticket resumed across multiple agent
// sessions keeps all of them available for retrospect, unlike the
// single-value Session field MarkDoneWithMetadata's doc comment describes as
// dropped.
func AppendSessionID(path, sessionID string) error {
	return updateTicket(path, func(t *schema.Ticket) {
		t.SessionIDs = append(t.SessionIDs, sessionID)
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
