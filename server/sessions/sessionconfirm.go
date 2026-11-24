package sessions

import (
	"fmt"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// confirmSettleTimeout caps the wait for Claude's end-of-turn line: it reports
// idle before it flushes the turn's last lines, a failed first message among
// them. A turn cut short may never write one.
const confirmSettleTimeout = 2 * time.Second

const confirmSettlePoll = 50 * time.Millisecond

// firstReplyReader reads the first reply after a switch for one harness.
type firstReplyReader interface {
	// Path finds the transcript or rollout to read.
	Path(mg *Manager, id string, meta Meta) (string, bool)
	// Settle waits until the turn's last lines are written. It returns false
	// when cancel or a stop ended the wait.
	Settle(rs *Resumer, path string, cancel <-chan struct{}) bool
	// StateOK reports whether a session in state may be checked.
	StateOK(state string) bool
	// FirstReply reads the first reply stamped after at. found is false while
	// there is none.
	FirstReply(path string, at time.Time) (notFound, found bool, err error)
}

func firstReplyReaderFor(h string) firstReplyReader {
	if h == string(harness.Codex) {
		return codexReplyReader{}
	}
	return claudeReplyReader{}
}

type claudeReplyReader struct{}

func (claudeReplyReader) Path(mg *Manager, id string, meta Meta) (string, bool) {
	return mg.chat.ClaudeTranscript(id, meta.ChatMeta())
}

func (claudeReplyReader) Settle(rs *Resumer, path string, cancel <-chan struct{}) bool {
	return rs.settleClaudeTurn(path, cancel)
}

func (claudeReplyReader) StateOK(state string) bool { return state == wire.SessionIdle }

func (claudeReplyReader) FirstReply(path string, at time.Time) (notFound, found bool, err error) {
	e, isErr, found, err := transcript.FirstAssistantAfter(path, at)
	return isErr && e.Error == transcript.ModelNotFound, found, err
}

type codexReplyReader struct{}

func (codexReplyReader) Path(mg *Manager, _ string, meta Meta) (string, bool) {
	return mg.chat.CodexRollout(meta.ChatMeta())
}

func (codexReplyReader) Settle(*Resumer, string, <-chan struct{}) bool { return true }

// StateOK: Codex reports a failed first turn as Blocked, a clean one as Idle.
func (codexReplyReader) StateOK(state string) bool {
	return state == wire.SessionIdle || state == wire.SessionBlocked
}

func (codexReplyReader) FirstReply(path string, at time.Time) (notFound, found bool, err error) {
	kind, found, err := codexsession.FirstTurnEndAfter(path, at)
	return kind == transcript.ModelNotFound, found, err
}

// confirmSwitch decides an unconfirmed switch by the first reply after the
// switch time: Claude's first assistant line, Codex's first task_complete.
// model_not_found rolls the switch back; any other reply confirms it; none
// keeps the window open. cancel ends the wait when the session turns busy,
// since the check runs again when that turn ends.
func (rs *Resumer) confirmSwitch(dir, id string, run int, cancel <-chan struct{}) {
	mg := rs.mg
	meta, err := ReadMeta(dir)
	if err != nil || meta.Run != run || !meta.SwitchUnconfirmed {
		return
	}
	reader := firstReplyReaderFor(meta.Harness)
	path, ok := reader.Path(mg, id, meta)
	if !ok || !reader.Settle(rs, path, cancel) {
		return
	}
	switch verdict, rb := rs.decideSwitch(dir, id, run, reader, path); verdict {
	case verdictConfirmed:
		mg.events.Publish(wire.EventSessionChanged, id)
	case verdictRollBack:
		rs.finishRollback(dir, id, rb)
	}
}

type switchVerdict int

const (
	verdictNone switchVerdict = iota
	verdictConfirmed
	verdictRollBack
)

// decideSwitch reads the first reply under metaMu and applies the locked half
// of the verdict: it saves a confirmation, or starts a rollback.
func (rs *Resumer) decideSwitch(dir, id string, run int, reader firstReplyReader, path string) (switchVerdict, rollback) {
	mg := rs.mg
	mg.Watches.MetaMu.Lock()
	defer mg.Watches.MetaMu.Unlock()
	meta, err := ReadMeta(dir)
	if err != nil || meta.Run != run || !meta.SwitchUnconfirmed || !reader.StateOK(meta.Status.State) {
		return verdictNone, rollback{}
	}
	notFound, found, err := reader.FirstReply(path, meta.SwitchedAt)
	if err != nil {
		mg.log.Warn("switch: read first reply", "session", id, "err", err)
	}
	switch {
	case err != nil || !found:
		return verdictNone, rollback{}
	case notFound && meta.SwitchPrev != nil:
		rb, ok := rs.beginRollback(dir, meta)
		if !ok {
			return verdictNone, rollback{}
		}
		return verdictRollBack, rb
	}
	meta.SwitchUnconfirmed, meta.SwitchPrev = false, nil
	if err := WriteMeta(dir, meta); err != nil {
		mg.log.Warn("switch: confirm", "session", id, "err", err)
		return verdictNone, rollback{}
	}
	return verdictConfirmed, rollback{}
}

// settleClaudeTurn waits for the end-of-turn line of the transcript at path. It
// returns false when cancel or a stop ended the wait.
func (rs *Resumer) settleClaudeTurn(path string, cancel <-chan struct{}) bool {
	deadline := time.NewTimer(confirmSettleTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(confirmSettlePoll)
	defer tick.Stop()
	for {
		if ended, err := transcript.TurnEnded(path); err != nil || ended {
			return true
		}
		select {
		case <-cancel:
			return false
		case <-rs.mg.Watches.Stop:
			return false
		case <-deadline.C:
			return true
		case <-tick.C:
		}
	}
}

// rollback is the model switched away from and the record to return to.
type rollback struct {
	attempted string
	prev      SwitchTarget
}

// beginRollback bumps the run and registers the pending switch back, with the
// state set at detection so the prompt box is disabled meanwhile. The caller
// holds metaMu and meta is the saved record.
func (rs *Resumer) beginRollback(dir string, meta Meta) (rollback, bool) {
	mg := rs.mg
	rb := rollback{attempted: meta.Model, prev: *meta.SwitchPrev}
	meta.Run++
	if err := WriteMeta(dir, meta); err != nil {
		mg.log.Warn("switch: roll back", "session", meta.ID, "err", err)
		return rollback{}, false
	}
	mg.switches.set(meta.ID, switchState{run: meta.Run + 1, model: rb.prev.Model, effort: rb.prev.Effort, back: true, attempted: rb.attempted})
	return rb, true
}

// finishRollback relaunches the conversation on the record from before an
// unconfirmed switch, with a new switch time so the failed turn reads as older.
// It runs without metaMu while the old agent stops.
func (rs *Resumer) finishRollback(dir, id string, rb rollback) {
	mg := rs.mg
	rs.stopForRestart(dir, id)
	mg.Watches.MetaMu.Lock()
	defer mg.Watches.MetaMu.Unlock()
	meta, err := ReadMeta(dir)
	if err != nil || meta.Status.State == wire.SessionExited {
		mg.switches.clear(id)
		return
	}
	prev := rb.prev
	meta.Model, meta.Effort, meta.SwitchedByHand = prev.Model, prev.Effort, prev.ByHand
	meta.SwitchUnconfirmed, meta.SwitchPrev, meta.SwitchedAt = false, nil, time.Now()
	if err := rs.rollbackSwitchLocked(dir, &meta, rb.attempted); err != nil {
		mg.log.Warn("switch: old agent did not start", "session", id, "err", err)
	}
}

// dismissSession clears a delivery-failure card. It judges the saved meta, not
// Info(), so a card replaced since the caller's read is not cleared by mistake;
// Info() is consulted only to tell a derived or blocked card from no card.
func dismissSession(mg *Manager, req wire.SessionRequest) (wire.SessionResult, error) {
	dir, _, res, err := mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	var info wire.SessionInfo
	var refusal wire.SessionResult
	if err := mg.UpdateSessionMeta(dir, req.ID, func(m *Meta) bool {
		info = m.Info()
		switch {
		case m.Actionable.IsDelivery():
			m.Actionable = wire.SessionActionable{}
			info = m.Info()
			return true
		case info.Actionable.Reason != "":
			refusal = Refusal(wire.ReasonSessionNotDeliveryCard, fmt.Sprintf("session %s shows a different card; answer or retry it instead of dismissing it", req.ID))
		default:
			refusal = Refusal(wire.ReasonSessionNoCard, fmt.Sprintf("session %s has no card to dismiss", req.ID))
		}
		return false
	}); err != nil {
		return wire.SessionResult{}, err
	}
	if refusal.Refused {
		return refusal, nil
	}
	return wire.SessionResult{Session: &info}, nil
}
