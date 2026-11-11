package sessions

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/nativerunner/holder"
)

// StartContinue runs a design Continue's action in the background: the
// charting handoff when the session is due one, else the typed prompts. The
// caller holds metaMu; the work runs after it lets go.
func (rs *Resumer) StartContinue(dir string, meta Meta, label string, action []string) {
	if rs.chartingHandoffDue(meta, action) {
		rs.mg.Watches.Start(func() { rs.chartingHandoff(dir, meta.ID) })
		return
	}
	rs.mg.Watches.Start(func() { rs.mg.prompter.SendAction(dir, label, action) })
}

// chartingHandoffDue reports whether action is the design Continue of a
// strongest-tier session that gx launched for the design, on a map past
// charting. Only a harness with a chat reader restarts, as only it has a design Continue.
func (rs *Resumer) chartingHandoffDue(meta Meta, action []string) bool {
	return meta.LaunchDesign && meta.IsDesign() &&
		rs.mg.settings.Load().HasChatReader(meta.Harness) &&
		meta.Tier == agent.TierStrongest &&
		slices.Equal(action, chat.ContinueAction(meta.FlowRef(), meta.Harness)) &&
		rs.mg.Maps.DesignLaunchTier(meta.Ticket) == agent.TierWorker
}

// HandoffRefusal is why a Continue of action must not start: it is a charting
// handoff whose worker-tier harness is not on PATH. It runs before the run-end
// line is cleared and the old agent stopped, so the session stays as it was.
func (rs *Resumer) HandoffRefusal(meta Meta, action []string) (wire.SessionResult, bool) {
	if !rs.chartingHandoffDue(meta, action) {
		return wire.SessionResult{}, false
	}
	msg := rs.mg.harness.InteractiveMissingReason(meta.Harness, "the session's harness")
	if msg == "" {
		return wire.SessionResult{}, false
	}
	return Refusal(wire.ReasonSessionHarnessMissing, msg), true
}

// chartingHandoff ends the session's strongest-tier conversation and starts a
// fresh one under the worker tier in the same gx session, which then types
// /gx-design as its first prompt. Bumping Run first makes the old watch's
// exit a no-op, so it does not end the session. A restart that fails ends the
// session Exited crashed, as any failed start does; Resume carries on.
func (rs *Resumer) chartingHandoff(dir, id string) {
	old, ok := rs.beginRestart(dir, id)
	if !ok {
		return
	}
	rs.stopForRestart(dir, id)
	rs.mg.Watches.MetaMu.Lock()
	defer rs.mg.Watches.MetaMu.Unlock()
	meta, err := ReadMeta(dir)
	if err != nil {
		rs.mg.log.Warn("handoff: read meta", "session", id, "err", err)
		return
	}
	if err := rs.restartOnWorker(dir, &meta, old); err != nil {
		rs.mg.log.Warn("handoff: restart", "session", id, "err", err)
		rs.mg.endFailedStart(dir, &meta)
	}
}

// beginRestart bumps the run and returns the conversation being ended. It
// reports false when the session ended in the meantime.
func (rs *Resumer) beginRestart(dir, id string) (string, bool) {
	m := rs.mg
	m.Watches.MetaMu.Lock()
	defer m.Watches.MetaMu.Unlock()
	meta, err := ReadMeta(dir)
	if err != nil || meta.Status.State == wire.SessionExited {
		return "", false
	}
	meta.Run++
	if err := WriteMeta(dir, meta); err != nil {
		m.log.Warn("restart: save meta", "session", id, "err", err)
		return "", false
	}
	return meta.ConversationID, true
}

// stopForRestart stops the old holder and waits for it to save its exit. The
// caller holds no lock.
func (rs *Resumer) stopForRestart(dir, id string) {
	m := rs.mg
	c, err := holder.DialHolder(dir)
	if err != nil {
		return
	}
	if err := c.Stop(); err != nil {
		m.log.Warn("restart: stop agent", "session", id, "err", err)
		return
	}
	waitForFile(filepath.Join(dir, holder.ExitCodeFile), sessionStopTimeout)
}

// restartOnWorker ends the old conversation like an agent exit, records a new
// one and launches it under the worker tier. The caller holds metaMu.
func (rs *Resumer) restartOnWorker(dir string, meta *Meta, old string) error {
	return rs.restartConversation(dir, meta, old, func(meta *Meta, _ int) {
		applyTier(rs.mg, meta, agent.TierWorker)
		meta.HandoffTier = true
		meta.FirstPrompt = harness.SkillCommand(meta.Harness, "gx-design", chat.TicketEpic(meta.Ticket))
	})
}

// restartConversation is the holder restart both restarts share: it ends the
// old conversation like an agent exit, records a new one, lets change set what
// the new launch differs in (it receives the run the launch will have, so no
// caller guesses it), and starts a fresh holder and watch. The caller holds
// metaMu.
func (rs *Resumer) restartConversation(dir string, meta *Meta, old string, change func(meta *Meta, newRun int)) error {
	m := rs.mg
	m.Conv.SaveTotals(meta.ConvSession(), old)
	conv, err := nativerunner.NewUUID()
	if err != nil {
		return err
	}
	meta.ConversationID = conv
	change(meta, meta.Run+1)
	meta.Actionable = wire.SessionActionable{}
	m.Conv.Record(meta.ConvSession())
	if err := clearOldHolder(dir); err != nil {
		return err
	}
	meta.Run++
	meta.Status, meta.StatusChangedAt, meta.Exit, meta.Stopped = wire.SessionStatus{}, time.Now(), "", false
	if err := m.startSessionHolder(dir, meta, 0, 0, false); err != nil {
		return err
	}
	id, run := meta.ID, meta.Run
	m.Watches.Start(func() { m.watchSession(dir, id, run, time.Time{}, false) })
	m.events.Publish(wire.EventSessionChanged, id)
	return nil
}

// RestartFresh restarts a live session in the same tier with a new run and a
// new conversation, and types prompt again as its first prompt (meta clears it
// once handed over, so the caller hands it back). It records the run it made in
// VimRelaunchRun so a caller restarts once per launch. The caller must not hold
// metaMu. A restart that fails ends the session as a failed start does.
func (rs *Resumer) RestartFresh(dir, id, prompt string) {
	m := rs.mg
	old, ok := rs.beginRestart(dir, id)
	if !ok {
		return
	}
	rs.stopForRestart(dir, id)
	m.Watches.MetaMu.Lock()
	defer m.Watches.MetaMu.Unlock()
	meta, err := ReadMeta(dir)
	if err != nil {
		m.log.Warn("restart: read meta", "session", id, "err", err)
		return
	}
	if err := rs.restartConversation(dir, &meta, old, func(meta *Meta, newRun int) {
		meta.FirstPrompt = prompt
		meta.VimRelaunchRun = newRun
	}); err != nil {
		m.log.Warn("restart: restart", "session", id, "err", err)
		m.endFailedStart(dir, &meta)
	}
}
