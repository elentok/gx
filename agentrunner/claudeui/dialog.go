package claudeui

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
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

	var options []string
	for _, row := range rows[q+1 : esc] {
		t := strings.TrimSpace(row)
		if t == "" {
			continue
		}
		m := optionRow.FindStringSubmatch(t)
		if m == nil || m[1] != strconv.Itoa(len(options)+1) {
			return Dialog{}
		}
		options = append(options, strings.TrimSpace(m[2]))
	}
	if len(options) == 0 {
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

const planQuestionPrefix = "Claude has written up a plan"

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

	var options []string
	cursor := -1
	for _, row := range rows[q+1:] {
		t := strings.TrimSpace(row)
		if t == "" {
			continue
		}
		m := optionRow.FindStringSubmatch(t)
		if m == nil || m[1] != strconv.Itoa(len(options)+1) {
			break
		}
		if strings.HasPrefix(t, "❯") {
			cursor = len(options)
		}
		options = append(options, strings.TrimSpace(m[2]))
	}
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

// isDashRule reports whether row is a ╌ separator around a diff or command.
func isDashRule(row string) bool {
	return strings.Contains(row, "╌╌╌") && strings.Trim(row, "╌ ") == ""
}
