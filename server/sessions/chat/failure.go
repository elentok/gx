package chat

import (
	"cmp"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

const stalledLabel = "No progress for 15 min"

// SavedTurn is the model and effort the session's record holds, with no time.
func (m Meta) SavedTurn() codexsession.TurnModel {
	return codexsession.TurnModel{Model: m.Model, Effort: m.Effort}
}

// StatusFailure is the failure that status alone decides.
func (m Meta) StatusFailure() *wire.SessionFailure {
	switch m.Status.State {
	case wire.SessionExited:
		if m.Exit == wire.SessionExitCrashed {
			return &wire.SessionFailure{Kind: wire.FailureKindCrashed, Label: "Agent crashed", Recrashed: m.RestoreCrashed}
		}
		return &wire.SessionFailure{Kind: wire.FailureKindEnded, Label: "Session ended"}
	case wire.SessionRateLimited:
		return &wire.SessionFailure{Kind: wire.FailureKindRateLimit, Label: "Rate limited", Text: m.Status.Message}
	case wire.SessionBlocked:
		if m.Status.Kind == string(harness.BlockedError) {
			return &wire.SessionFailure{Kind: wire.FailureKindBlocked, Why: m.Status.Kind, Label: "Agent hit an error", Text: m.Status.Message}
		}
		return &wire.SessionFailure{Kind: wire.FailureKindBlocked, Why: m.Status.Kind, Label: "Waiting at a prompt in the terminal"}
	}
	return nil
}

// ReportedModel is the model ID on the newest assistant message of a Claude
// session's transcript, or the saved one before a reply. A Codex session reads
// its rollout's newest turn instead, see ReportedCodexTurn.
func (c *Chat) ReportedModel(m Meta) string {
	model, _ := c.ReportedModelAndContext(m)
	return model
}

// ReportedModelAndContext is ReportedModel plus the context tokens of the
// newest real assistant reply, from the same cached transcript read. tokens is
// zero when there is no reading (always for harnesses other than Claude and Codex).
func (c *Chat) ReportedModelAndContext(m Meta) (model string, tokens int) {
	if m.Harness == string(harness.Codex) {
		turn, tokens := c.ReportedCodex(m)
		return turn.Model, tokens
	}
	turn, tokens := c.ReportedClaudeTurn(m)
	return turn.Model, tokens
}

// ReportedCodexTurn is the model and effort on the newest turn of a Codex
// session's rollout, or the saved ones while there is no rollout or turn.
func (c *Chat) ReportedCodexTurn(m Meta) codexsession.TurnModel {
	turn, _ := c.ReportedCodex(m)
	return turn
}

// CodexRollout is the rollout file of a Codex session's conversation.
func (c *Chat) CodexRollout(m Meta) (string, bool) {
	home, err := c.src.CodexHome()
	if err != nil {
		return "", false
	}
	lookup := codexsession.RolloutPathIn
	if c.rolloutPath != nil {
		lookup = c.rolloutPath
	}
	path, ok, err := lookup(home, m.ConversationID)
	if err != nil {
		c.log.Warn("find codex rollout", "session", m.ID, "err", err)
	}
	return path, ok && err == nil
}

// ReportedCodex is ReportedCodexTurn plus the context tokens of the rollout's
// newest token_count (zero while there is no rollout or reading), behind one
// rollout path lookup. Callers wanting both use it, not the two reads apart.
func (c *Chat) ReportedCodex(m Meta) (codexsession.TurnModel, int) {
	saved := m.SavedTurn()
	home, err := c.src.CodexHome()
	if err != nil {
		return saved, 0
	}
	lookup := codexsession.RolloutPathIn
	if c.rolloutPath != nil {
		lookup = c.rolloutPath
	}
	path, ok, err := lookup(home, m.ConversationID)
	if err != nil || !ok {
		return saved, 0
	}
	turn, err := c.codexTurns.Read(path, func(p string) (codexsession.TurnModel, error) {
		t, _, err := codexsession.ReadTurnModel(p)
		return t, err
	})
	if err != nil {
		c.log.Warn("read codex turn model", "session", m.ID, "err", err)
	}
	tokens, err := c.codexContext.Read(path, func(p string) (int, error) {
		t, _, err := codexsession.ReadContextTokens(p)
		return t, err
	})
	if err != nil {
		c.log.Warn("read codex context tokens", "session", m.ID, "err", err)
	}
	return codexsession.TurnModel{Model: cmp.Or(turn.Model, m.Model), Effort: cmp.Or(turn.Effort, m.Effort), At: turn.At}, tokens
}

// ShownModel is the model and effort a session shows. Until a turn newer than
// switchedAt reports in, they are the record's (saved), since the reported ones
// still describe the model before the switch. After that the reported model
// shows, and the reported effort where the harness reports one (Codex), else the
// record's. A zero switchedAt, or a reading with no time, shows the reported
// ones as before.
func ShownModel(saved, reported codexsession.TurnModel, switchedAt time.Time, reportsEffort bool) codexsession.TurnModel {
	if switchedAt.IsZero() {
		return reported
	}
	if !reported.At.After(switchedAt) {
		return saved
	}
	if !reportsEffort {
		reported.Effort = saved.Effort
	}
	return reported
}

// ReportedClaudeTurn is the model and time of a Claude session's newest
// assistant message (the saved model, no time, before a reply), plus the
// context tokens, from the one cached transcript read.
func (c *Chat) ReportedClaudeTurn(m Meta) (codexsession.TurnModel, int) {
	saved := m.SavedTurn()
	if m.Harness != string(harness.Claude) {
		return saved, 0
	}
	path, ok := c.ClaudeTranscript(m.ID, m)
	if !ok {
		return saved, 0
	}
	usage, err := c.reportedUsage.Read(path, func(p string) (transcript.Usage, error) {
		usage, _, err := transcript.LastAssistantReading(p)
		return usage, err
	})
	if err != nil {
		c.log.Warn("read reported model", "session", m.ID, "err", err)
	}
	return codexsession.TurnModel{Model: cmp.Or(usage.Model, m.Model), Effort: m.Effort, At: usage.At}, usage.Occupancy()
}

// ModelSwitched reports that the model shown is not the one the launch tier
// chose. Claude launches with an alias and replies with a full id, so the alias
// inside the reply counts as the same model.
func ModelSwitched(launched, shown string) bool {
	if launched == "" || shown == "" {
		return false
	}
	return !strings.Contains(strings.ToLower(shown), strings.ToLower(launched))
}

// Failure is the failure status decides, else the one a transcript read does.
func (c *Chat) Failure(m Meta) *wire.SessionFailure {
	if f := m.StatusFailure(); f != nil {
		return f
	}
	return c.transcriptFailure(m)
}

// transcriptFailure is the auth or stalled failure of a session on the Claude
// transcript format (Claude, or the chat-harness test seam). Codex errors and
// stalls are read elsewhere.
func (c *Chat) transcriptFailure(m Meta) *wire.SessionFailure {
	state := m.Status.State
	if state != wire.SessionIdle && state != wire.SessionWorking || m.Harness == string(harness.Codex) || !c.src.HasChatReader(m.Harness) {
		return nil
	}
	path, ok := c.ClaudeTranscript(m.ID, m)
	if !ok {
		return nil
	}
	switch state {
	case wire.SessionIdle:
		auth, err := c.authFailures.Read(path, func(p string) (bool, error) {
			e, ok, err := transcript.LastAPIError(p)
			return ok && (e.Status == 401 || e.Error == "authentication_error"), err
		})
		if err == nil && auth {
			return &wire.SessionFailure{Kind: wire.FailureKindAuth, Label: "Claude login needed", Text: "Run `claude /login` on this computer, then Retry."}
		}
	case wire.SessionWorking:
		// A background task outlives the turn; the session waits, it is not stalled.
		if m.Status.Background {
			break
		}
		if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) >= c.cfg.StallThreshold {
			return &wire.SessionFailure{Kind: wire.FailureKindStalled, Label: stalledLabel}
		}
	}
	return nil
}
