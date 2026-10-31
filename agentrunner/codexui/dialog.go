// Package codexui reads Codex's approval dialog off the screen, for the case
// where the PermissionRequest hook did not fire.
package codexui

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/elentok/gx/agentrunner/claudeui"
)

// The dialog text and the keys are from Codex 0.166.0's wording as the spec
// records it and as fakecodex draws it; no real dialog was ever captured. They
// live here only, so the live smoke checklist and `gx codex doctor` re-check
// one place.
const (
	// Title is the line that opens the dialog; the message is below it.
	Title = "Blocked by hook"
	// AllowKey approves. DenyKey cancels, and the model then waits for a prompt.
	AllowKey = "y"
	DenyKey  = "\x1b"
)

// ParseDialog reads the approval dialog off the screen rows. Anything it does
// not recognise, or a dialog with no message to show, is not approvable: gx
// never answers a screen it cannot read.
func ParseDialog(rows []string) claudeui.Dialog {
	title := -1
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(rows[i]), Title) {
			title = i
			break
		}
	}
	if title < 0 {
		return claudeui.Dialog{}
	}
	var body []string
	for _, row := range rows[title+1:] {
		t := strings.TrimSpace(row)
		if t == "" {
			if len(body) > 0 {
				break
			}
			continue
		}
		body = append(body, t)
	}
	d := claudeui.Dialog{Header: Title, Body: claudeui.Normalize(body...)}
	if len(body) > 0 {
		d.Approvable = true
		sum := sha256.Sum256([]byte(d.Header + "\n" + d.Body))
		d.ID = hex.EncodeToString(sum[:8])
	}
	return d
}
