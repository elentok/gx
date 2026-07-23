// Package tickets parses and loads the local markdown issue tracker under
// `.scratch/` (see .ai's issue-tracker-local skill for the on-disk
// conventions), mirroring the role the git package plays for ui/prs.
package tickets

import (
	"regexp"
	"strconv"
	"strings"
)

// Ticket is one parsed `<epic>/issues/NN-<slug>.md` file. Number, Title, and
// Path are filled in by the loader from the filename/path, not by ParseTicket
// (which only ever sees the file's raw text).
type Ticket struct {
	Number int
	Title  string
	Path   string

	Type      string
	BlockedBy []int
	Status    string // raw Status: value; "" means missing (valid open/unclaimed default)
	Body      string // raw markdown after the leading metadata lines, unmodified
}

var doneStatuses = map[string]bool{
	"done":       true,
	"resolved":   true,
	"wontfix":    true,
	"closed":     true,
	"superseded": true,
}

// IsDone reports whether the ticket's raw Status collapses into the "done"
// family. Used for epic open/total counts; the full five-state rendered
// status (open/claimed/blocked/needs-info/done) is a later concern.
func (t Ticket) IsDone() bool {
	return doneStatuses[strings.ToLower(strings.TrimSpace(t.Status))]
}

var metadataLineRe = regexp.MustCompile(`(?i)^(Type|Blocked by|Status):\s*(.*)$`)

var blockedByNumberRe = regexp.MustCompile(`\d+`)

// ParseTicket parses a ticket file's raw text into leading metadata (Type:,
// Blocked by:, Status:) plus the remaining raw markdown body. A missing
// Status: line is the valid open/unclaimed default, not an error.
func ParseTicket(raw string) (Ticket, error) {
	var t Ticket

	lines := strings.Split(raw, "\n")
	i := 0
	for ; i < len(lines); i++ {
		m := metadataLineRe.FindStringSubmatch(lines[i])
		if m == nil {
			break
		}
		key := strings.ToLower(m[1])
		value := strings.TrimSpace(m[2])
		switch key {
		case "type":
			t.Type = value
		case "blocked by":
			t.BlockedBy = parseBlockedBy(value)
		case "status":
			t.Status = value
		}
	}

	t.Body = strings.Join(lines[i:], "\n")
	return t, nil
}

// parseBlockedBy extracts ticket numbers from a "Blocked by:" value, e.g.
// "02, 05" -> [2, 5]. A value with no digits (e.g. "-" or "None") yields nil.
func parseBlockedBy(value string) []int {
	matches := blockedByNumberRe.FindAllString(value, -1)
	if len(matches) == 0 {
		return nil
	}
	nums := make([]int, 0, len(matches))
	for _, m := range matches {
		n, err := strconv.Atoi(m)
		if err != nil {
			continue
		}
		nums = append(nums, n)
	}
	return nums
}
