package agentfake

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/harness"
)

// TurnEnded is nil when the fake's turn is over: replies agent messages are in
// the chat and its pid file says idle. The fake sets busy before it appends the
// reply and idle in a delayed Finish, so both mean the Finish ran. Call it after
// the poll that saw the reply, never before, or an idle from before the turn
// passes. The error names the pid-file status and the reply count.
func TurnEnded(env []string, conversationID string, replies int) error {
	status, found := PidStatusFor(env, conversationID)
	if !found {
		status = "no pid file"
	}
	if replies > 0 && status == StatusIdle {
		return nil
	}
	return fmt.Errorf("fake pid-file status %q, %d agent replies", status, replies)
}

// PidStatusFor returns the status in the pid file whose sessionId is
// sessionID, read from the sessions directory the fake's env (HOME,
// CLAUDE_CONFIG_DIR) names, not the caller's own. The file is keyed by OS pid,
// so it scans the directory. found is false when no file names the session;
// status is empty while the fake is starting.
func PidStatusFor(env []string, sessionID string) (status string, found bool) {
	dir, err := harness.ClaudeSessionsDirIn(env)
	if err != nil {
		return "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var p PidFile
		if json.Unmarshal(raw, &p) == nil && p.SessionID == sessionID {
			return p.Status, true
		}
	}
	return "", false
}
