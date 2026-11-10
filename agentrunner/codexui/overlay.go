package codexui

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/elentok/gx/agentrunner/claudeui"
)

// NoneLabel is the row Codex appends to every question's options.
const NoneLabel = "None of the above"

// Overlay is Codex's request_user_input overlay as read off the screen.
type Overlay struct {
	Number, Total int
	Unanswered    int
	Question      string
	// Labels holds every numbered row's label, the trailing "None of the
	// above" included, so Rows == len(Labels).
	Labels []string
	Rows   int
	// Cursor is the 1-based row carrying the › marker.
	Cursor int
	// LastQuestion is true when the footer says "enter to submit all".
	LastQuestion bool
	// NotesOpen is true when a "› ..." notes line sits under the rows.
	NotesOpen bool
}

var (
	overlayHeader = regexp.MustCompile(`^Question (\d+)/(\d+) \((\d+) unanswered\)$`)
	overlayRow    = regexp.MustCompile(`^(›?)\s*(\d+)\. (.+)$`)
	overlayGap    = regexp.MustCompile(`\s{2,}`)
)

// ParseOverlay reads the overlay off the screen rows. The last header on the
// screen wins, so a stale one above a redraw is ignored. Any layout it does
// not fully recognise returns ok=false: gx never sends keys to a guess.
func ParseOverlay(rows []string) (Overlay, bool) {
	header := -1
	var m []string
	for i := len(rows) - 1; i >= 0; i-- {
		if m = overlayHeader.FindStringSubmatch(strings.TrimSpace(rows[i])); m != nil {
			header = i
			break
		}
	}
	if header < 0 {
		return Overlay{}, false
	}
	o := Overlay{}
	o.Number, _ = strconv.Atoi(m[1])
	o.Total, _ = strconv.Atoi(m[2])
	o.Unanswered, _ = strconv.Atoi(m[3])
	if o.Number < 1 || o.Number > o.Total {
		return Overlay{}, false
	}

	var question []string
	footer := ""
	inRows := false
	for _, raw := range rows[header+1:] {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if strings.Contains(t, "enter to submit") {
			footer = t
			break
		}
		if rm := overlayRow.FindStringSubmatch(t); rm != nil && rm[2] == strconv.Itoa(len(o.Labels)+1) {
			inRows = true
			if rm[1] == "›" {
				if o.Cursor != 0 {
					return Overlay{}, false
				}
				o.Cursor = len(o.Labels) + 1
			}
			o.Labels = append(o.Labels, overlayGap.Split(rm[3], 2)[0])
			continue
		}
		switch {
		case !inRows:
			question = append(question, t)
		case strings.HasPrefix(t, "›"):
			o.NotesOpen = true
		}
		// Other text after the rows is a wrapped description.
	}

	switch {
	case strings.Contains(footer, "enter to submit all"):
		o.LastQuestion = true
	case strings.Contains(footer, "enter to submit answer"):
	default:
		return Overlay{}, false
	}
	o.Question = claudeui.Normalize(question...)
	o.Rows = len(o.Labels)
	if o.Question == "" || o.Rows < 2 || o.Cursor == 0 || o.Labels[o.Rows-1] != NoneLabel {
		return Overlay{}, false
	}
	return o, true
}
