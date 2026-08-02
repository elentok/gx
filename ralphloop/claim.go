package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

var statusLineRe = regexp.MustCompile(`(?i)^Status:\s*(.*)$`)
var metadataKeyRe = regexp.MustCompile(`(?i)^(Type|Blocked by|Status):\s*(.*)$`)

// Claim writes "Status: claimed" into the ticket file at path.
func Claim(path string) error {
	return SetStatus(path, "claimed")
}

// MarkDone writes "Status: done" into the ticket file at path.
func MarkDone(path string) error {
	return SetStatus(path, "done")
}

// MarkNeedsInfo writes "Status: needs-info" into the ticket file at path.
func MarkNeedsInfo(path string) error {
	return SetStatus(path, "needs-info")
}

// MarkNeedsAttention writes "Status: needs-attention" into the ticket file.
func MarkNeedsAttention(path string) error {
	return SetStatus(path, "needs-attention")
}

// MarkDoneWithMetadata marks a ticket done and, right after the Status line,
// records the closing iteration's final context-window occupancy and agent
// session id, matching the Status line's plain/bold style. This lets a human
// retroactively open the exact Claude Code session that produced the
// ticket's work, without separate log correlation. Only meant for the case
// where a fresh iteration's own session id is already in hand at close time
// (see finishIteration) — a reattached close with no live session should
// call MarkDone instead. Status and the two metadata lines land in a single
// atomic write, so a concurrent reader never observes Status: done without
// them. Requires an existing Status: line — every ticket reaching this point
// was already claimed, so this is a precondition, not an edge case to
// recover from. For a frontmatter-format ticket, the session id isn't
// written (the spec drops that field from frontmatter) and contextWindow
// lands in actual_context_window — see the schema.HasFrontmatter branch
// below.
func MarkDoneWithMetadata(path string, contextWindow int, sessionID string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rawStr := string(raw)

	if schema.HasFrontmatter(rawStr) {
		// contextWindow lands in actual_context_window — per
		// .scratch/ticket-frontmatter/spec.md, that field unifies and
		// replaces the old Context window: line, which already recorded
		// the same contextOccupancy-at-close measurement. The spec drops
		// the old Session: field from frontmatter entirely (the
		// iteration's session id is already durably recorded in
		// run-log.jsonl via logTicketEvent), so sessionID isn't written
		// here.
		return updateFrontmatterTicket(path, rawStr, func(t *schema.Ticket) {
			t.Status = schema.StatusDone
			t.ActualContextWindow = contextWindow
		})
	}

	lines := strings.Split(rawStr, "\n")

	idx, bold := findStatusLine(lines)
	if idx < 0 {
		return fmt.Errorf("no Status: line found in %s", path)
	}
	lines[idx] = formatMetadataLine("Status", "done", bold)
	lines = insertLines(lines, idx+1,
		formatMetadataLine("Context window", strconv.Itoa(contextWindow), bold),
		formatMetadataLine("Session", sessionID, bold),
	)

	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")))
}

// updateFrontmatterTicket is the shared round-trip behind every
// frontmatter-format status writer below: parse raw via schema's typed
// parser, let mutate apply the caller's field changes, then marshal and
// write back atomically. Centralizing this keeps every writer's YAML block
// valid, rather than each one line-splicing its own edit into it.
func updateFrontmatterTicket(path, raw string, mutate func(*schema.Ticket)) error {
	t, err := schema.ParseTicketFromRaw(raw, path)
	if err != nil {
		return err
	}
	mutate(&t)

	out, err := schema.MarshalTicket(t, schema.ParseBody(raw))
	if err != nil {
		return err
	}
	return writeFileAtomic(path, out)
}

func formatMetadataLine(key, value string, bold bool) string {
	if bold {
		return fmt.Sprintf("**%s:** %s", key, value)
	}
	return fmt.Sprintf("%s: %s", key, value)
}

// insertLines splices new lines into lines starting at index at.
func insertLines(lines []string, at int, new ...string) []string {
	return append(lines[:at:at], append(new, lines[at:]...)...)
}

// SetStatus rewrites (or adds) a ticket file's Status: line to value,
// leaving the rest of the file's content byte-for-byte unchanged. It
// recognizes both plain (`Status:`) and bold-markdown (`**Status:**`)
// styles, matching tickets.ParseTicket, and preserves whichever style was
// already in use. If the file has no Status: line yet, the new line is
// inserted right after the last Type:/Blocked by: metadata line (matching
// that line's style), or after the title line if there's no metadata at
// all.
func SetStatus(path, value string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rawStr := string(raw)

	if schema.HasFrontmatter(rawStr) {
		return updateFrontmatterTicket(path, rawStr, func(t *schema.Ticket) {
			t.Status = schema.Status(value)
		})
	}

	lines := strings.Split(rawStr, "\n")

	if idx, bold := findStatusLine(lines); idx >= 0 {
		lines[idx] = formatMetadataLine("Status", value, bold)
	} else {
		insertAt, bold := findMetadataInsertPoint(lines)
		insertion := []string{formatMetadataLine("Status", value, bold)}
		if insertAt >= len(lines) || strings.TrimSpace(lines[insertAt]) != "" {
			// Keep the file's blank-line-separated metadata style: only the
			// no-metadata-at-all case (title directly followed by a blank
			// line) skips this, since there's nothing to separate from yet.
			insertion = append(insertion, "")
		}
		lines = insertLines(lines, insertAt, insertion...)
	}

	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")))
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

// findStatusLine returns the index of lines' existing Status: line (-1 if
// none) and whether it used the bold-markdown style.
func findStatusLine(lines []string) (idx int, bold bool) {
	for i, line := range lines {
		stripped := strings.ReplaceAll(line, "**", "")
		if statusLineRe.MatchString(stripped) {
			return i, strings.Contains(line, "**")
		}
	}
	return -1, false
}

// findMetadataInsertPoint returns where a new Status: line should go when
// none exists yet: right after the last Type:/Blocked by: line's paragraph
// (skipping one trailing blank line, if present) matching that line's style;
// or, with no metadata at all, in the same spot right after the title line's
// paragraph, so the new line lands as its own paragraph rather than glued to
// the title.
func findMetadataInsertPoint(lines []string) (idx int, bold bool) {
	if len(lines) == 0 {
		return 0, false
	}

	anchor := 0 // falls back to the title line when there's no metadata
	anchorBold := false
	for i, line := range lines {
		stripped := strings.ReplaceAll(line, "**", "")
		if metadataKeyRe.MatchString(stripped) {
			anchor = i
			anchorBold = strings.Contains(line, "**")
		}
	}

	insertAt := anchor + 1
	if insertAt < len(lines) && strings.TrimSpace(lines[insertAt]) == "" {
		insertAt++
	}
	return insertAt, anchorBold
}
