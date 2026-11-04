package server

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
)

// startContinue runs a design Continue's action in the background: the
// charting handoff when the session is due one, else the typed prompts. The
// caller holds metaMu; the work runs after it lets go.
func (rs *Resumer) startContinue(dir string, meta sessionMeta, label string, action []string) {
	if rs.chartingHandoffDue(meta, action) {
		rs.mg.watches.start(func() { rs.chartingHandoff(dir, meta.ID) })
		return
	}
	rs.mg.watches.start(func() { rs.mg.prompter.sendAction(dir, label, action) })
}

// chartingHandoffDue reports whether action is the design Continue of a
// strongest-tier session that gx launched for the design, on a map past
// charting. Only a harness with a chat reader restarts, as only it has a design Continue.
func (rs *Resumer) chartingHandoffDue(meta sessionMeta, action []string) bool {
	return meta.LaunchDesign && meta.Design &&
		rs.mg.settings.Load().HasChatReader(meta.Harness) &&
		meta.Tier == ralphloop.TierStrongest &&
		slices.Equal(action, chat.DesignContinueAction(meta.Ticket, meta.Harness)) &&
		rs.mg.designLaunchTier(meta.Ticket) == ralphloop.TierWorker
}

// handoffRefusal is why a Continue of action must not start: it is a charting
// handoff whose worker-tier harness is not on PATH. It runs before the run-end
// line is cleared and the old agent stopped, so the session stays as it was.
func (rs *Resumer) handoffRefusal(meta sessionMeta, action []string) (wire.SessionResult, bool) {
	if !rs.chartingHandoffDue(meta, action) {
		return wire.SessionResult{}, false
	}
	msg := rs.mg.harness.InteractiveMissingReason(meta.Harness, "the session's harness")
	if msg == "" {
		return wire.SessionResult{}, false
	}
	return sessionRefusal(wire.ReasonSessionHarnessMissing, msg), true
}

// chartingHandoff ends the session's strongest-tier conversation and starts a
// fresh one under the worker tier in the same gx session, which then types
// /gx-design as its first prompt. Bumping Run first makes the old watch's
// exit a no-op, so it does not end the session. A restart that fails ends the
// session Exited crashed, as any failed start does; Resume carries on.
func (rs *Resumer) chartingHandoff(dir, id string) {
	old, ok := rs.beginHandoff(dir, id)
	if !ok {
		return
	}
	// No lock held: the stop waits for the holder to save its exit.
	if c, err := nativerunner.DialHolder(dir); err == nil {
		if err := c.Stop(); err != nil {
			rs.mg.log.Warn("handoff: stop agent", "session", id, "err", err)
		} else {
			waitForFile(filepath.Join(dir, nativerunner.ExitCodeFile), sessionStopTimeout)
		}
	}
	rs.mg.watches.metaMu.Lock()
	defer rs.mg.watches.metaMu.Unlock()
	meta, err := readSessionMeta(dir)
	if err != nil {
		rs.mg.log.Warn("handoff: read meta", "session", id, "err", err)
		return
	}
	if err := rs.restartOnWorker(dir, &meta, old); err != nil {
		rs.mg.log.Warn("handoff: restart", "session", id, "err", err)
		rs.mg.endFailedStart(dir, &meta)
	}
}

// beginHandoff bumps the run and returns the conversation being ended. It
// reports false when the session ended in the meantime.
func (rs *Resumer) beginHandoff(dir, id string) (string, bool) {
	rs.mg.watches.metaMu.Lock()
	defer rs.mg.watches.metaMu.Unlock()
	meta, err := readSessionMeta(dir)
	if err != nil || meta.Status.State == wire.SessionExited {
		return "", false
	}
	meta.Run++
	if err := writeSessionMeta(dir, meta); err != nil {
		rs.mg.log.Warn("handoff: save meta", "session", id, "err", err)
		return "", false
	}
	return meta.ConversationID, true
}

// restartOnWorker ends the old conversation like an agent exit, records a new
// one and launches it under the worker tier. The caller holds metaMu.
func (rs *Resumer) restartOnWorker(dir string, meta *sessionMeta, old string) error {
	rs.mg.saveConversationTotals(*meta, old)
	conv, err := nativerunner.NewUUID()
	if err != nil {
		return err
	}
	meta.ConversationID = conv
	rs.mg.applyTier(meta, ralphloop.TierWorker)
	meta.HandoffTier = true
	meta.FirstPrompt = harness.SkillCommand(meta.Harness, "gx-design", chat.TicketEpic(meta.Ticket))
	meta.Actionable = wire.SessionActionable{}
	rs.mg.recordConversation(*meta)
	if err := clearOldHolder(dir); err != nil {
		return err
	}
	meta.Run++
	meta.Status, meta.StatusChangedAt, meta.Exit, meta.Stopped = wire.SessionStatus{}, time.Now(), "", false
	if err := rs.mg.startSessionHolder(dir, meta, 0, 0, false); err != nil {
		return err
	}
	id, run := meta.ID, meta.Run
	rs.mg.watches.start(func() { rs.mg.watchSession(dir, id, run, time.Time{}, false) })
	rs.mg.events.Publish(wire.EventSessionChanged, id)
	return nil
}
