package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const vimInsertMarker = "-- INSERT --"

// ReadyToType makes sure claude's input is in insert mode when the editor mode
// could be vim (see probeWanted). A missing marker is never read as NORMAL: the
// probe presses `i` and judges by what the input did. false means unknown; the
// caller must not type.
func (h claudeHarness) ReadyToType(io TypingIO) bool {
	if !probeWanted(io.Session, h.env, io) {
		return true
	}
	return probeVim(io.Session, io)
}

func (claudeHarness) InputPresses(session string) int { return InputPresses(session) }

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
