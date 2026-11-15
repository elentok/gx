package sessions

import (
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// confirmSettleTimeout caps the wait for Claude's end-of-turn line: it reports
// idle before it flushes the turn's last lines, a failed first message among
// them. A turn cut short may never write one.
const confirmSettleTimeout = 2 * time.Second

const confirmSettlePoll = 50 * time.Millisecond

// modelNotFound is the error code of a reply from a model that does not exist
// or is not available to the account. Claude does not exit on it.
const modelNotFound = "model_not_found"

// confirmSwitch decides an unconfirmed Claude switch by the first assistant line
// after the switch time. model_not_found rolls the switch back; any other line
// confirms it; none keeps the window open. cancel ends the wait when the
// session turns busy, since the check runs again when that turn ends.
func (rs *Resumer) confirmSwitch(dir, id string, run int, cancel <-chan struct{}) {
	mg := rs.mg
	meta, err := ReadMeta(dir)
	if err != nil || meta.Run != run || !meta.SwitchUnconfirmed || meta.Harness == string(harness.Codex) {
		return
	}
	path, ok := mg.chat.ClaudeTranscript(id, meta.ChatMeta())
	if !ok {
		return
	}
	deadline := time.NewTimer(confirmSettleTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(confirmSettlePoll)
	defer tick.Stop()
wait:
	for {
		if ended, err := transcript.TurnEnded(path); err != nil || ended {
			break
		}
		select {
		case <-cancel:
			return
		case <-mg.Watches.Stop:
			return
		case <-deadline.C:
			break wait
		case <-tick.C:
		}
	}
	mg.Watches.MetaMu.Lock()
	meta, err = ReadMeta(dir)
	if err != nil || meta.Run != run || !meta.SwitchUnconfirmed || meta.Status.State != wire.SessionIdle {
		mg.Watches.MetaMu.Unlock()
		return
	}
	e, isErr, found, err := transcript.FirstAssistantAfter(path, meta.SwitchedAt)
	if err != nil {
		mg.log.Warn("switch: read first reply", "session", id, "err", err)
	}
	switch {
	case err != nil || !found:
		mg.Watches.MetaMu.Unlock()
	case isErr && e.Error == modelNotFound && meta.SwitchPrev != nil:
		rs.rollbackUnconfirmed(dir, meta)
	default:
		meta.SwitchUnconfirmed, meta.SwitchPrev = false, nil
		err := WriteMeta(dir, meta)
		mg.Watches.MetaMu.Unlock()
		if err != nil {
			mg.log.Warn("switch: confirm", "session", id, "err", err)
			return
		}
		mg.events.Publish(wire.EventSessionChanged, id)
	}
}

// rollbackUnconfirmed relaunches the conversation on the record from before an
// unconfirmed switch, with a new switch time so the failed turn reads as older.
// The caller holds metaMu and meta is the saved record; it is released here
// while the old agent stops. The state is set at detection so the prompt box
// is disabled meanwhile.
func (rs *Resumer) rollbackUnconfirmed(dir string, meta Meta) {
	mg := rs.mg
	id, attempted, prev := meta.ID, meta.Model, *meta.SwitchPrev
	meta.Run++
	if err := WriteMeta(dir, meta); err != nil {
		mg.Watches.MetaMu.Unlock()
		mg.log.Warn("switch: roll back", "session", id, "err", err)
		return
	}
	mg.switches.set(id, switchState{run: meta.Run + 1, model: prev.Model, effort: prev.Effort, back: true, attempted: attempted})
	mg.Watches.MetaMu.Unlock()
	rs.stopForRestart(dir, id)
	mg.Watches.MetaMu.Lock()
	defer mg.Watches.MetaMu.Unlock()
	meta, err := ReadMeta(dir)
	if err != nil || meta.Status.State == wire.SessionExited {
		mg.switches.clear(id)
		return
	}
	meta.Model, meta.Effort, meta.SwitchedByHand = prev.Model, prev.Effort, prev.ByHand
	meta.SwitchUnconfirmed, meta.SwitchPrev, meta.SwitchedAt = false, nil, time.Now()
	if err := rs.rollbackSwitchLocked(dir, &meta, attempted); err != nil {
		mg.log.Warn("switch: old agent did not start", "session", id, "err", err)
	}
}
