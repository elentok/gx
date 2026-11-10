package claudeui

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
)

// AskCursor says which row of an AskDialog the ❯ cursor is on.
type AskCursor string

const (
	AskCursorOption  AskCursor = "option"  // AskDialog.Options[Index]
	AskCursorAdvance AskCursor = "advance" // the unnumbered Next/Submit row
	AskCursorChat    AskCursor = "chat"    // "Chat about this"
)

// AskTab is one question tab in the dialog's tab bar.
type AskTab struct {
	Label    string
	Answered bool // ☒ rather than ☐
}

// AskOption is one numbered row of the question list.
type AskOption struct {
	Label       string // first row of the label
	Description string // indented rows under the label, including wrapped label rows
	Checked     bool   // multi-select [✔]
	// Other is the free-text row ("Type something."); once typed its Label is
	// the typed text. It is always the last numbered option.
	Other bool
}

// AskReviewItem is one answered question on the review screen.
type AskReviewItem struct {
	Question string
	Answer   string
}

// AskDialog is a parsed Claude AskUserQuestion screen: one question, or the
// review screen. Parsing sends nothing.
type AskDialog struct {
	Tabs        []AskTab
	SubmitTab   bool // the "✔ Submit" tab is in the bar
	Review      bool
	Question    string
	Multi       bool
	Options     []AskOption // on the review screen: "Submit answers", "Cancel"
	Advance     string      // "Next" or "Submit" on a multi-select, else ""
	ChatNumber  int         // digit of "Chat about this", 0 when absent
	Cursor      AskCursor
	CursorIndex int // option index when Cursor is AskCursorOption
	ReviewItems []AskReviewItem
	ID          string // hash of the question block, stable across answers and cursor moves
}

var (
	askNumbered = regexp.MustCompile(`^(❯)?\s*(\d+)\.\s+(.*)$`)
	askCheckbox = regexp.MustCompile(`^\[( |✔)\]\s*(.*)$`)
	askScroll   = regexp.MustCompile(`^[↑↓]\s`)
	askTabSplit = regexp.MustCompile(`\s{2,}`)
)

const (
	askReviewTitle  = "Review your answers"
	askReviewPrompt = "Ready to submit your answers?"
	askChatLabel    = "Chat about this"
)

// ParseAskDialog reads the AskUserQuestion dialog off the screen rows. ok is
// false for any layout it cannot read with certainty: no tab bar, an unknown
// footer, rows hidden behind ↑/↓ scroll markers, or no ❯ row in view.
func ParseAskDialog(rows []string) (d AskDialog, ok bool) {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = askClean(r)
	}

	foot := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "Enter to select") {
			foot = i
			break
		}
	}
	if foot < 0 || !askFooterKnown(lines[foot:]) {
		return AskDialog{}, false
	}

	bar := -1
	for i := 0; i < foot; i++ {
		if strings.HasPrefix(strings.TrimLeft(lines[i], "← "), "☐") || strings.HasPrefix(strings.TrimLeft(lines[i], "← "), "☒") {
			bar = i
			break
		}
	}
	if bar < 0 || !d.readTabs(lines[bar]) {
		return AskDialog{}, false
	}

	body := lines[bar+1 : foot]
	cursorSeen := false
	for _, l := range body {
		if askScroll.MatchString(l) {
			return AskDialog{}, false
		}
		if strings.HasPrefix(l, "❯") {
			cursorSeen = true
		}
	}
	if !cursorSeen {
		return AskDialog{}, false
	}

	if strings.Contains(strings.Join(body, "\n"), askReviewTitle) {
		ok = d.readReview(body)
	} else {
		ok = d.readQuestion(body)
	}
	if !ok || d.Cursor == "" {
		return AskDialog{}, false
	}
	d.ID = d.id()
	return d, true
}

// askClean drops the "│ " gutter claude draws at narrow widths and the
// surrounding space.
func askClean(row string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(row), "│"))
}

// askFooterKnown accepts the footer, wrapped over several rows, and the extra
// hints claude appends ("ctrl+g to edit in Vim", the auto-continue countdown).
func askFooterKnown(rows []string) bool {
	text := Normalize(rows...)
	return strings.HasPrefix(text, "Enter to select") && strings.Contains(text, "Esc to cancel")
}

func (d *AskDialog) readTabs(bar string) bool {
	for _, tok := range askTabSplit.Split(strings.Trim(bar, "← →"), -1) {
		switch {
		case tok == "✔ Submit":
			d.SubmitTab = true
		case strings.HasPrefix(tok, "☐ "), strings.HasPrefix(tok, "☒ "):
			d.Tabs = append(d.Tabs, AskTab{Label: strings.TrimSpace(tok[len("☐"):]), Answered: strings.HasPrefix(tok, "☒")})
		default:
			return false
		}
	}
	return len(d.Tabs) > 0
}

// readOption reads a numbered row expected to carry number n.
func readOption(l string, n int) (label string, cursor, ok bool) {
	m := askNumbered.FindStringSubmatch(l)
	if m == nil || m[2] != strconv.Itoa(n) {
		return "", false, false
	}
	return strings.TrimSpace(m[3]), m[1] != "", true
}

func (d *AskDialog) readQuestion(body []string) bool {
	var question []string
	i := 0
	for ; i < len(body); i++ {
		if _, _, ok := readOption(body[i], 1); ok {
			break
		}
		if body[i] != "" {
			question = append(question, body[i])
		}
	}
	d.Question = Normalize(question...)
	if d.Question == "" || i == len(body) {
		return false
	}

	var desc []string
	flush := func() {
		if n := len(d.Options); n > 0 {
			d.Options[n-1].Description = Normalize(desc...)
		}
		desc = nil
	}
	rule := -1
	for ; i < len(body) && rule < 0; i++ {
		l := body[i]
		if label, cur, ok := readOption(l, len(d.Options)+1); ok {
			flush()
			opt := AskOption{Label: label}
			if m := askCheckbox.FindStringSubmatch(label); m != nil {
				opt.Checked, opt.Label = m[1] == "✔", m[2]
				d.Multi = len(d.Options) == 0 || d.Multi
			}
			d.Options = append(d.Options, opt)
			if cur {
				d.Cursor, d.CursorIndex = AskCursorOption, len(d.Options)-1
			}
			continue
		}
		switch t := strings.TrimSpace(strings.TrimPrefix(l, "❯")); {
		case l == "":
		case isRule(l):
			rule = i
		case d.Multi && len(desc) == 0 && (t == "Next" || t == "Submit"):
			d.Advance = t
			if strings.HasPrefix(l, "❯") {
				d.Cursor = AskCursorAdvance
			}
		default:
			desc = append(desc, t)
		}
	}
	flush()
	if len(d.Options) == 0 {
		return false
	}
	d.Options[len(d.Options)-1].Other = true

	if rule < 0 {
		return true
	}
	for _, l := range body[rule+1:] {
		if l == "" {
			continue
		}
		label, cur, ok := readOption(l, len(d.Options)+1)
		if !ok || label != askChatLabel || d.ChatNumber != 0 {
			return false
		}
		d.ChatNumber = len(d.Options) + 1
		if cur {
			d.Cursor = AskCursorChat
		}
	}
	return true
}

func (d *AskDialog) readReview(body []string) bool {
	d.Review = true
	var item *AskReviewItem
	inAnswer := false
	i := 0
	for ; i < len(body) && !strings.HasPrefix(body[i], askReviewPrompt); i++ {
		switch t := body[i]; {
		case t == "" || t == askReviewTitle:
		case strings.HasPrefix(t, "●"):
			d.ReviewItems = append(d.ReviewItems, AskReviewItem{Question: Normalize(strings.TrimPrefix(t, "●"))})
			item, inAnswer = &d.ReviewItems[len(d.ReviewItems)-1], false
		case item == nil:
			return false
		case strings.HasPrefix(t, "→"):
			item.Answer, inAnswer = Normalize(strings.TrimPrefix(t, "→")), true
		case inAnswer:
			item.Answer = Normalize(item.Answer, t)
		default:
			item.Question = Normalize(item.Question, t)
		}
	}
	if i == len(body) {
		return false
	}
	d.Question = askReviewPrompt
	for _, l := range body[i+1:] {
		if l == "" {
			continue
		}
		label, cur, ok := readOption(l, len(d.Options)+1)
		if !ok {
			return false
		}
		d.Options = append(d.Options, AskOption{Label: label})
		if cur {
			d.Cursor, d.CursorIndex = AskCursorOption, len(d.Options)-1
		}
	}
	return len(d.Options) == 2 && d.Options[0].Label == "Submit answers" && d.Options[1].Label == "Cancel"
}

// id hashes the question block: tab labels, question, option labels and
// descriptions. Answers, checkmarks, the typed Other text and the cursor are
// left out so the id survives them.
func (d *AskDialog) id() string {
	parts := []string{strconv.FormatBool(d.Review), d.Question}
	for _, t := range d.Tabs {
		parts = append(parts, t.Label)
	}
	for _, o := range d.Options {
		if o.Other && !d.Review {
			parts = append(parts, "other")
			continue
		}
		parts = append(parts, o.Label, o.Description)
	}
	for _, it := range d.ReviewItems {
		parts = append(parts, it.Question)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}
