package claudeui

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// approvableHeaders are the dialog titles gx may approve from the web UI.
// Anything else (Read file, plan approval, AskUserQuestion, new tools) is
// shown without detail and answered in the terminal.
var approvableHeaders = map[string]bool{
	"Bash command": true,
	"Edit file":    true,
	"Create file":  true,
}

// optionRow matches a numbered dialog option, with or without the ❯ cursor.
var optionRow = regexp.MustCompile(`^(?:❯\s*)?(\d+)\.\s+(.*)$`)

// Dialog is a parsed permission dialog. The zero value means the rows show no
// recognizable dialog.
type Dialog struct {
	Header     string // title line, for example "Bash command"
	Body       string // normalized text between the title and the question
	Question   string // the "Do you want ...?" line
	Options    []string
	Approvable bool
	// ID is a hash of Header and Body only, so it survives a changed cwd line,
	// tip line, footer and terminal width. Empty unless Approvable.
	ID string
}

// ParseDialog reads claude's permission dialog off the screen rows. It
// returns a not-approvable Dialog for anything it does not recognize rather
// than guessing: the layout is not a stable contract and changes between
// claude releases.
func ParseDialog(rows []string) Dialog {
	esc := -1
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(rows[i]), "Esc to cancel") {
			esc = i
			break
		}
	}
	if esc < 0 {
		return Dialog{}
	}
	q := -1
	for i := esc - 1; i >= 0; i-- {
		t := strings.TrimSpace(rows[i])
		if strings.HasPrefix(t, "Do you want") && strings.HasSuffix(t, "?") {
			q = i
			break
		}
	}
	if q < 0 {
		return Dialog{}
	}

	options, _, clean, wrapped := readOptions(rows[q+1 : esc])
	if !clean || wrapped || len(options) == 0 {
		return Dialog{}
	}

	header, body := headerAndBody(rows, q)
	d := Dialog{
		Header:   header,
		Body:     body,
		Question: Normalize(rows[q]),
		Options:  options,
	}
	if approvableHeaders[header] && options[0] == "Yes" {
		d.Approvable = true
		d.ID = dialogID(header, body)
	}
	return d
}

// readOptions reads the numbered options at the top of rows. A row indented to
// the label column of the option above, with no number, is that option's
// wrapped label and joins it. The first row that is neither stops the list and
// clean is false; blank rows are skipped.
func readOptions(rows []string) (options []string, cursor int, clean, wrapped bool) {
	cursor = -1
	textCol := 0
	for _, row := range rows {
		t := strings.TrimSpace(row)
		if t == "" {
			continue
		}
		indent := utf8.RuneCountInString(row) - utf8.RuneCountInString(strings.TrimLeft(row, " \t"))
		if m := optionRow.FindStringSubmatchIndex(t); m != nil && t[m[2]:m[3]] == strconv.Itoa(len(options)+1) {
			if strings.HasPrefix(t, "❯") {
				cursor = len(options)
			}
			textCol = indent + utf8.RuneCountInString(t[:m[4]])
			options = append(options, strings.TrimSpace(t[m[4]:]))
			continue
		}
		if len(options) > 0 && indent >= textCol && !isRule(t) && !isDashRule(t) && !isHintRow(t) {
			options[len(options)-1] = Normalize(options[len(options)-1], t)
			wrapped = true
			continue
		}
		return options, cursor, false, wrapped
	}
	return options, cursor, true, wrapped
}

// isHintRow reports whether t is a key hint claude prints under an option,
// which is not part of the label.
func isHintRow(t string) bool {
	return strings.HasPrefix(t, "shift+tab ") || strings.HasPrefix(t, "ctrl+")
}

// headerAndBody reads the title and body between the last rule above the
// question row q and the question.
func headerAndBody(rows []string, q int) (header, body string) {
	start := 0
	for i := q - 1; i >= 0; i-- {
		if isRule(rows[i]) {
			start = i + 1
			break
		}
	}
	var lines []string
	for _, row := range rows[start:q] {
		t := strings.TrimSpace(row)
		switch {
		case t == "", isDashRule(t), strings.HasPrefix(t, "Tip:"), strings.HasPrefix(t, "cwd:"):
		case header == "":
			header = Normalize(t)
		default:
			lines = append(lines, t)
		}
	}
	return header, Normalize(lines...)
}

func dialogID(header, body string) string {
	sum := sha256.Sum256([]byte(header + "\n" + body))
	return hex.EncodeToString(sum[:8])
}

// DialogKind says what a dialog read by ReadDialog asks.
type DialogKind string

const (
	DialogUnreadable DialogKind = ""
	DialogPermission DialogKind = "permission"
	DialogPlan       DialogKind = "plan"
)

// DialogView is any numbered-option dialog on screen, whatever its header.
// The zero value (Kind DialogUnreadable) means no such dialog was found.
type DialogView struct {
	Kind     DialogKind
	Header   string
	Body     string
	Question string
	Options  []string
	// Cursor is the 0-based index of the option the ❯ cursor is on, or -1.
	Cursor int
	// PlainYes is true when option 1 is exactly "Yes".
	PlainYes bool
	// ID hashes Header and Body, like Dialog.ID. Empty when unreadable.
	ID string
}

// PlanQuestionPrefix starts the question line of a plan approval dialog.
const PlanQuestionPrefix = "Claude has written up a plan"

const planQuestionPrefix = PlanQuestionPrefix

func isQuestionRow(t string) bool {
	return (strings.HasPrefix(t, "Do you want") && strings.HasSuffix(t, "?")) ||
		(strings.HasPrefix(t, planQuestionPrefix) && strings.HasSuffix(t, "?"))
}

// ReadDialog reads the last question line on screen plus the numbered options
// under it. Unlike ParseDialog it needs no "Esc to cancel" row and applies no
// header limit; it never decides whether a dialog may be approved.
func ReadDialog(rows []string) DialogView {
	q := -1
	for i := len(rows) - 1; i >= 0; i-- {
		if isQuestionRow(strings.TrimSpace(rows[i])) {
			q = i
			break
		}
	}
	if q < 0 {
		return DialogView{}
	}

	options, cursor, _, _ := readOptions(rows[q+1:])
	if len(options) == 0 {
		return DialogView{}
	}

	header, body := headerAndBody(rows, q)
	kind := DialogPermission
	if strings.HasPrefix(strings.TrimSpace(rows[q]), planQuestionPrefix) {
		kind = DialogPlan
	}
	return DialogView{
		Kind:     kind,
		Header:   header,
		Body:     body,
		Question: Normalize(rows[q]),
		Options:  options,
		Cursor:   cursor,
		PlainYes: options[0] == "Yes",
		ID:       dialogID(header, body),
	}
}

// PlanFilePath is the plan file the footer of a plan dialog names ("ctrl+g to
// edit in Vim · ~/.claude/plans/x.md"), or "" with none. A narrow screen wraps
// the path onto the row below.
func PlanFilePath(rows []string) string {
	for i := len(rows) - 1; i >= 0; i-- {
		t := strings.TrimSpace(rows[i])
		if !strings.HasPrefix(t, "ctrl+g to edit in") {
			continue
		}
		_, path, _ := strings.Cut(t, "·")
		path = strings.TrimSpace(path)
		if path == "" && i+1 < len(rows) {
			path = strings.TrimSpace(rows[i+1])
		}
		return path
	}
	return ""
}

// isDashRule reports whether row is a ╌ separator around a diff or command.
func isDashRule(row string) bool {
	return strings.Contains(row, "╌╌╌") && strings.Trim(row, "╌ ") == ""
}
