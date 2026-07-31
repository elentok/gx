package ralphloop

import (
	"os"
	"regexp"
	"strings"
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
	lines := strings.Split(string(raw), "\n")

	if idx, bold := findStatusLine(lines); idx >= 0 {
		lines[idx] = formatStatusLine(value, bold)
	} else {
		insertAt, bold := findMetadataInsertPoint(lines)
		insertion := []string{formatStatusLine(value, bold)}
		if insertAt >= len(lines) || strings.TrimSpace(lines[insertAt]) != "" {
			// Keep the file's blank-line-separated metadata style: only the
			// no-metadata-at-all case (title directly followed by a blank
			// line) skips this, since there's nothing to separate from yet.
			insertion = append(insertion, "")
		}
		lines = append(lines[:insertAt:insertAt], append(insertion, lines[insertAt:]...)...)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func formatStatusLine(value string, bold bool) string {
	if bold {
		return "**Status:** " + value
	}
	return "Status: " + value
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
