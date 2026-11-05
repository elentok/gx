package wire

import (
	"fmt"
	"strings"
)

// MaxSessionPromptBytes caps the text one send types.
const MaxSessionPromptBytes = 32 * 1024

// NormalizeAnswer turns every line ending into \n so a CR cannot submit the
// prompt early.
func NormalizeAnswer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// ValidatePromptText normalizes text typed into a session and refuses a
// bang prefix and an oversized text. Empty text passes: each route
// decides whether that is allowed. what names the text in refusal messages.
func ValidatePromptText(text, what string) (string, *SessionResult) {
	text = strings.TrimSpace(NormalizeAnswer(text))
	switch {
	case strings.HasPrefix(text, "!"):
		res := SessionResult{Refused: true, Reason: ReasonPromptPrefixNotAllowed, Message: what + " may not start with !"}
		return text, &res
	case len(text) > MaxSessionPromptBytes:
		res := SessionResult{Refused: true, Reason: ReasonPromptTextTooLong, Message: fmt.Sprintf("%s is over %d bytes", what, MaxSessionPromptBytes)}
		return text, &res
	}
	return text, nil
}
