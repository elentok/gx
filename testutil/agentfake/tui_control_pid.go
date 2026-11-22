package agentfake

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/sessions"
)

// TurnEnded is nil when the fake's turn is over: replies agent messages are in
// the chat and its pid file says idle. The fake sets busy before it appends the
// reply and idle in a delayed Finish, so both mean the Finish ran. Call it after
// the poll that saw the reply, never before, or an idle from before the turn
// passes. The error names the pid-file status and the reply count.
func TurnEnded(env []string, conversationID string, replies int) error {
	if replies > 0 && pidStatusText(env, conversationID) == StatusIdle {
		return nil
	}
	return errors.New(TurnState(env, conversationID, replies))
}

// TurnState says what the fake's pid file shows and how many agent replies the
// caller saw, for a timeout error.
func TurnState(env []string, conversationID string, replies int) string {
	return fmt.Sprintf("fake pid-file status %q, %d agent replies", pidStatusText(env, conversationID), replies)
}

// metaConversationID reads the conversation id from the gx session dir's meta.
func metaConversationID(sessionDir string) (string, error) {
	m, err := sessions.ReadMeta(sessionDir)
	if err == nil && m.ConversationID == "" {
		err = errors.New("no conversation id yet")
	}
	return m.ConversationID, err
}

// TurnEndedIn is TurnEnded for the gx session whose meta lives in sessionDir.
// An unreadable meta or an empty conversation id is returned as the error.
func TurnEndedIn(env []string, sessionDir string, replies int) error {
	conv, err := metaConversationID(sessionDir)
	if err != nil {
		return err
	}
	return TurnEnded(env, conv, replies)
}

// TurnTimeoutIn wraps a wait's err with TurnState, or with the meta error when
// the conversation id cannot be read.
func TurnTimeoutIn(err error, env []string, sessionDir string, replies int) error {
	conv, merr := metaConversationID(sessionDir)
	if merr != nil {
		return fmt.Errorf("%w (%v, %d agent replies)", err, merr, replies)
	}
	return fmt.Errorf("%w (%s)", err, TurnState(env, conv, replies))
}

func pidStatusText(env []string, conversationID string) string {
	status, found := PidStatusFor(env, conversationID)
	if !found {
		return "no pid file"
	}
	return status
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
