package server

import (
	"cmp"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// Kinds of SessionFailure. Not named SessionFailure*: tygo would read them as
// the struct's own enum.
const (
	FailureKindCrashed   = "crashed"
	FailureKindEnded     = "ended"
	FailureKindAuth      = "auth"
	FailureKindRateLimit = "rate-limit"
	FailureKindStalled   = "stalled"
	FailureKindBlocked   = "blocked"
)

// SessionFailure is why a session needs the person, derived from its status
// and, for auth and stalled, from its transcript. It clears with its
// condition. Recrashed marks a crashed session whose Restore crashed again
// within restoreCrashWindow, so the page can stop offering Restore.
type SessionFailure struct {
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Text      string `json:"text,omitempty"`
	Recrashed bool   `json:"recrashed,omitempty"`
}

// restoreCrashWindow is how soon after a Restore a crash counts as the
// Restore failing. The auto-resume window, resumeCrashWindow, is another rule.
const restoreCrashWindow = time.Minute

// defaultStallThreshold is how long a Working session's transcript may stay
// untouched before the session counts as stalled.
const defaultStallThreshold = 15 * time.Minute

const stalledLabel = "No progress for 15 min"

// statusFailure is the failure that status alone decides.
func (m sessionMeta) statusFailure() *SessionFailure {
	switch m.Status.State {
	case SessionExited:
		if m.Exit == SessionExitCrashed {
			return &SessionFailure{Kind: FailureKindCrashed, Label: "Agent crashed", Recrashed: m.RestoreCrashed}
		}
		return &SessionFailure{Kind: FailureKindEnded, Label: "Session ended"}
	case SessionRateLimited:
		return &SessionFailure{Kind: FailureKindRateLimit, Label: "Rate limited", Text: m.Status.Message}
	case SessionBlocked:
		if m.Status.Kind == string(harness.BlockedError) {
			return &SessionFailure{Kind: FailureKindBlocked, Label: "Agent hit an error", Text: m.Status.Message}
		}
		return &SessionFailure{Kind: FailureKindBlocked, Label: "Waiting at a prompt in the terminal"}
	}
	return nil
}

// reportedModel is the model ID on the newest assistant message of a Claude
// session's transcript, or the saved one before a reply. A Codex session reads
// its rollout's newest turn instead, see reportedCodexTurn.
func (s *Server) reportedModel(m sessionMeta) string {
	if m.Harness == string(harness.Codex) {
		return s.reportedCodexTurn(m).Model
	}
	if m.Harness != string(harness.Claude) {
		return m.Model
	}
	path, ok := s.claudeTranscript(m.ID, m)
	if !ok {
		return m.Model
	}
	model, err := s.reportedModels.read(path, func(p string) (string, error) {
		usage, _, err := transcript.LastAssistantUsage(p)
		return usage.Model, err
	})
	if err != nil {
		s.log.Warn("read reported model", "session", m.ID, "err", err)
	}
	return cmp.Or(model, m.Model)
}

// reportedCodexTurn is the model and effort on the newest turn of a Codex
// session's rollout, or the saved ones while there is no rollout or turn.
func (s *Server) reportedCodexTurn(m sessionMeta) codexsession.TurnModel {
	saved := codexsession.TurnModel{Model: m.Model, Effort: m.Effort}
	home, err := s.codexHome()
	if err != nil {
		return saved
	}
	path, ok, err := codexsession.RolloutPathIn(home, m.ConversationID)
	if err != nil || !ok {
		return saved
	}
	turn, err := s.codexTurns.read(path, func(p string) (codexsession.TurnModel, error) {
		t, _, err := codexsession.ReadTurnModel(p)
		return t, err
	})
	if err != nil {
		s.log.Warn("read codex turn model", "session", m.ID, "err", err)
	}
	return codexsession.TurnModel{Model: cmp.Or(turn.Model, m.Model), Effort: cmp.Or(turn.Effort, m.Effort)}
}

// modelSwitched reports that the model shown is not the one the launch tier
// chose. Claude launches with an alias and replies with a full id, so the alias
// inside the reply counts as the same model.
func modelSwitched(launched, shown string) bool {
	if launched == "" || shown == "" {
		return false
	}
	return !strings.Contains(strings.ToLower(shown), strings.ToLower(launched))
}

// sessionInfo is m.info() plus the failure that needs a transcript read, the
// reported model and the running countdown.
func (s *Server) sessionInfo(m sessionMeta) SessionInfo {
	info := m.info()
	info.Model = s.reportedModel(m)
	info.ModelSwitched = modelSwitched(m.Model, info.Model)
	if m.Harness == string(harness.Codex) {
		info.Effort = s.reportedCodexTurn(m).Effort
	}
	if at := s.autoAdvances.deadline(m.ID); !at.IsZero() {
		info.AutoAdvanceAt = &at
	}
	info.Failure = m.statusFailure()
	if info.Failure == nil {
		info.Failure = s.transcriptFailure(m)
	}
	return info
}

// transcriptFailure is the auth or stalled failure of a session on the Claude
// transcript format (Claude, or the chat-harness test seam). Codex errors and
// stalls are read elsewhere.
func (s *Server) transcriptFailure(m sessionMeta) *SessionFailure {
	state := m.Status.State
	if state != SessionIdle && state != SessionWorking || m.Harness == string(harness.Codex) || !s.settings.Load().hasChatReader(m.Harness) {
		return nil
	}
	path, ok := s.claudeTranscript(m.ID, m)
	if !ok {
		return nil
	}
	switch state {
	case SessionIdle:
		auth, err := s.authFailures.read(path, func(p string) (bool, error) {
			e, ok, err := transcript.LastAPIError(p)
			return ok && (e.Status == 401 || e.Error == "authentication_error"), err
		})
		if err == nil && auth {
			return &SessionFailure{Kind: FailureKindAuth, Label: "Claude login needed", Text: "Run `claude /login` on this computer, then Retry."}
		}
	case SessionWorking:
		// A background task outlives the turn; the session waits, it is not stalled.
		if m.Status.Background {
			break
		}
		if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) >= s.cfg.StallThreshold {
			return &SessionFailure{Kind: FailureKindStalled, Label: stalledLabel}
		}
	}
	return nil
}
