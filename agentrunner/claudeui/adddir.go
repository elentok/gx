package claudeui

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// AddDirDialog is claude's "Adding a new directory" screen.
type AddDirDialog struct {
	Dir string // the directory it asks about
	// ID hashes the directory, so the same dialog keeps its id across cursor
	// moves.
	ID string
}

// ParseAddDirDialog reads the add-directory dialog off the screen rows. ok is
// false when both anchors are not on screen or no directory row follows the
// title. Parsing sends nothing.
func ParseAddDirDialog(rows []string) (AddDirDialog, bool) {
	if ClassifyStartup(Normalize(rows...)) != StartupAddDir {
		return AddDirDialog{}, false
	}
	for i, r := range rows {
		if !strings.Contains(r, addDirTitle) {
			continue
		}
		for _, next := range rows[i+1:] {
			if dir := strings.TrimSpace(next); dir != "" {
				sum := sha256.Sum256([]byte(dir))
				return AddDirDialog{Dir: dir, ID: "adddir-" + hex.EncodeToString(sum[:6])}, true
			}
		}
	}
	return AddDirDialog{}, false
}
