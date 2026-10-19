package claudeui

import "strings"

// gutterGlyphs sit between words when claude draws a bordered box or a
// message gutter, so they would split an anchor across a wrapped row.
var gutterGlyphs = strings.NewReplacer("│", " ", "⏺", " ")

// Normalize joins rows with a space, strips gutter glyphs and collapses
// whitespace, so text claude hard-wrapped across rows reads as one line.
func Normalize(rows ...string) string {
	return strings.Join(strings.Fields(gutterGlyphs.Replace(strings.Join(rows, " "))), " ")
}

// ScreenText is the text every screen rule matches: the scrollback tail plus
// the visible screen, normalized. Never match the raw PTY log: claude's
// cursor moves and redraws scatter a sentence across it.
func ScreenText(scrollbackTail, screen []string) string {
	return Normalize(append(append([]string{}, scrollbackTail...), screen...)...)
}

// Matches reports whether normalized screen text contains anchor. Keep
// anchors short and inside one paragraph so a narrow width can't split them
// around another element.
func Matches(text, anchor string) bool {
	return strings.Contains(text, Normalize(anchor))
}

// InputBox reports whether rows show claude's prompt input: a row starting
// with ❯ between two border rules. Dialog options also start with ❯ but sit
// under one rule at most, and an echoed prompt has no rule under it.
func InputBox(rows []string) bool {
	for i := 1; i+1 < len(rows); i++ {
		if strings.HasPrefix(strings.TrimSpace(rows[i]), "❯") && isRule(rows[i-1]) && isRule(rows[i+1]) {
			return true
		}
	}
	return false
}

// isRule reports whether row is a horizontal border: only ─ and spaces.
func isRule(row string) bool {
	row = strings.TrimSpace(row)
	return strings.Contains(row, "───") && strings.Trim(row, "─") == ""
}

// trustAnchors both show on claude's "trust this folder?" dialog; requiring
// both keeps a prompt that merely quotes one from passing for the dialog.
var trustAnchors = []string{
	"Quick safety check: Is this a project you created or one you trust?",
	trustYes,
}

const trustYes = "Yes, I trust this folder"

// TrustKey is the key that moves the trust dialog in rows toward trusting:
// Enter once the ❯ cursor is on the trust option, else Down. Claude selects
// "No, exit" first, so a bare Enter would quit it.
func TrustKey(rows []string) string {
	for _, row := range rows {
		if strings.Contains(row, "❯") {
			if Matches(Normalize(row), trustYes) {
				return "\r"
			}
			break
		}
	}
	return "\x1b[B"
}

// Startup is what a screen shows before claude writes its session pid file.
type Startup int

const (
	// StartupNoSession is any screen without a known pre-session dialog:
	// claude is still loading, or shows something gx can't answer (login,
	// onboarding), which a startup timeout reports.
	StartupNoSession Startup = iota
	// StartupTrust is the trust dialog, the one pre-session screen gx
	// answers by screen rule.
	StartupTrust
)

// ClassifyStartup reads a pre-session screen's normalized text.
func ClassifyStartup(text string) Startup {
	for _, a := range trustAnchors {
		if !Matches(text, a) {
			return StartupNoSession
		}
	}
	return StartupTrust
}
