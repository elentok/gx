package chat

import (
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// Transcript finds the conversation file of a session with a chat reader,
// and the format kind to read it with.
func (c *Chat) Transcript(id string, meta Meta) (string, agentrunner.Kind, bool) {
	if meta.Harness == string(harness.Codex) {
		path, ok := c.codexRollout(id, meta)
		return path, harness.Codex, ok
	}
	path, ok := c.ClaudeTranscript(id, meta)
	return path, harness.Claude, ok
}

// ForgetMiss drops a remembered transcript miss of conversationID.
func (c *Chat) ForgetMiss(conversationID string) { c.transcripts.ForgetMiss(conversationID) }

// codexArchive finds the zstd rollout of a Codex session whose plain rollout is
// gone. It only looks for the file; nothing is decoded.
func (c *Chat) codexArchive(id string, meta Meta) (string, bool) {
	if meta.Harness != string(harness.Codex) || meta.ConversationID == "" {
		return "", false
	}
	home, err := c.src.CodexHome()
	if err != nil {
		c.log.Warn("codex home", "session", id, "err", err)
		return "", false
	}
	path, ok, err := codexsession.ArchivedRolloutIn(home, meta.ConversationID)
	if err != nil {
		c.log.Warn("find archived rollout", "session", id, "err", err)
		return "", false
	}
	return path, ok
}

// codexRollout finds the rollout of a session's conversation, like
// ClaudeTranscript does for Claude.
func (c *Chat) codexRollout(id string, meta Meta) (string, bool) {
	if meta.ConversationID == "" {
		return "", false
	}
	return c.transcripts.Resolve(meta.ConversationID, meta.Status.State, time.Now(), c.cfg.TranscriptMissTTL, func() (string, bool) {
		home, err := c.src.CodexHome()
		if err != nil {
			c.log.Warn("codex home", "session", id, "err", err)
			return "", false
		}
		path, ok, err := codexsession.RolloutPathIn(home, meta.ConversationID)
		if err != nil {
			c.log.Warn("find rollout", "session", id, "err", err)
			return "", false
		}
		return path, ok
	})
}

// ClaudeTranscript finds the claude transcript of a session's conversation. A
// missing transcript (a codex session, or one not written yet) stays silent;
// real errors and an ambiguous id are logged.
func (c *Chat) ClaudeTranscript(id string, meta Meta) (string, bool) {
	conversationID := meta.ConversationID
	if conversationID == "" {
		return "", false
	}
	return c.transcripts.Resolve(conversationID, meta.Status.State, time.Now(), c.cfg.TranscriptMissTTL, func() (string, bool) {
		projects, err := harness.ClaudeProjectsDirIn(c.cfg.Env)
		if err != nil {
			c.log.Warn("claude projects dir", "session", id, "err", err)
			return "", false
		}
		// One stat of the expected slug dir; the glob over every project dir
		// is only for a session launched from a different cwd.
		expected := filepath.Join(projects, transcript.Slugify(meta.Dir), conversationID+".jsonl")
		if _, err := os.Stat(expected); err == nil {
			return expected, true
		}
		paths, err := transcript.FindByIDIn(projects, conversationID)
		if err != nil {
			c.log.Warn("find transcript", "session", id, "err", err)
			return "", false
		}
		if len(paths) > 1 {
			c.log.Warn("several transcripts", "session", id, "conversation", conversationID, "count", len(paths))
		}
		if len(paths) != 1 {
			return "", false
		}
		return paths[0], true
	})
}
