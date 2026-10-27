package harness

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	vimInsertMarker = "-- INSERT --"
	vimRecheck      = time.Second
	vimRecheckPoll  = 50 * time.Millisecond
)

// ReadyToType makes sure claude's input is in insert mode when vim mode is on:
// NORMAL mode shows no marker, so without the marker it presses `i` and
// re-checks for about a second. false means give-up; the caller must not type.
func (h claudeHarness) ReadyToType(io TypingIO) bool {
	if !claudeVimMode(h.env) {
		return true
	}
	if insertMarkerShown(io) {
		return true
	}
	slog.Info("claude vim mode has no INSERT marker, pressing i")
	if err := io.SendKeys("i"); err != nil {
		slog.Warn("claude vim mode: could not press i, not typing", "err", err)
		return false
	}
	for deadline := time.Now().Add(vimRecheck); time.Now().Before(deadline); {
		time.Sleep(vimRecheckPoll)
		if insertMarkerShown(io) {
			return true
		}
	}
	slog.Warn("claude vim mode: no INSERT marker after pressing i, not typing")
	return false
}

// insertMarkerShown looks for the marker in the footer rows below the last
// input row. A read error counts as no marker: typing into NORMAL mode is the
// worse mistake.
func insertMarkerShown(io TypingIO) bool {
	rows, err := io.Rows()
	if err != nil {
		return false
	}
	input := lastInputRow(rows, claudeInput)
	if input < 0 {
		return false
	}
	for _, row := range rows[input+1:] {
		if strings.Contains(plain(row), vimInsertMarker) {
			return true
		}
	}
	return false
}

// claudeVimMode re-reads claude's config on every call, so a mid-session
// /config toggle is caught. Either file may hold editorMode.
func claudeVimMode(env []string) bool {
	configDir, globalDir, err := claudeConfigDirs(env)
	if err != nil {
		return false
	}
	for _, path := range []string{
		filepath.Join(globalDir, ".claude.json"),
		filepath.Join(configDir, "settings.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			EditorMode string `json:"editorMode"`
		}
		if json.Unmarshal(data, &cfg) == nil && cfg.EditorMode == "vim" {
			return true
		}
	}
	return false
}
