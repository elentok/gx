package server

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
)

// startContinue runs a design Continue's action in the background: the
// charting handoff when the session is due one, else the typed prompts. The
// caller holds metaMu; the work runs after it lets go.
func (s *Server) startContinue(dir string, meta sessionMeta, label string, action []string) {
	if s.chartingHandoffDue(meta, action) {
		s.sessionWatches.start(func() { s.chartingHandoff(dir, meta.ID) })
		return
	}
	s.sessionWatches.start(func() { s.sendAction(dir, label, action) })
}

// chartingHandoffDue reports whether action is the design Continue of a
// strongest-tier session that gx launched for the design, on a map past
// charting. Only the chat harness restarts, as only it has a design Continue.
func (s *Server) chartingHandoffDue(meta sessionMeta, action []string) bool {
	return meta.LaunchDesign && meta.Design &&
		meta.Harness == s.settings.Load().chatHarness() &&
		meta.Tier == ralphloop.TierStrongest &&
		slices.Equal(action, designContinueAction(meta.Ticket, meta.Harness)) &&
		s.designLaunchTier(meta.Ticket) == ralphloop.TierWorker
}

// chartingHandoff ends the session's strongest-tier conversation and starts a
// fresh one under the worker tier in the same gx session, which then types
// /gx-design as its first prompt. Bumping Run first makes the old watch's
// exit a no-op, so it does not end the session. A restart that fails ends the
// session Exited crashed, as any failed start does; Resume carries on.
func (s *Server) chartingHandoff(dir, id string) {
	old, ok := s.beginHandoff(dir, id)
	if !ok {
		return
	}
	// No lock held: the stop waits for the holder to save its exit.
	if c, err := nativerunner.DialHolder(dir); err == nil {
		if err := c.Stop(); err != nil {
			s.log.Warn("handoff: stop agent", "session", id, "err", err)
		} else {
			waitForFile(filepath.Join(dir, nativerunner.ExitCodeFile), sessionStopTimeout)
		}
	}
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	meta, err := readSessionMeta(dir)
	if err != nil {
		s.log.Warn("handoff: read meta", "session", id, "err", err)
		return
	}
	if err := s.restartOnWorker(dir, &meta, old); err != nil {
		s.log.Warn("handoff: restart", "session", id, "err", err)
		s.endFailedStart(dir, &meta)
	}
}

// beginHandoff bumps the run and returns the conversation being ended. It
// reports false when the session ended in the meantime.
func (s *Server) beginHandoff(dir, id string) (string, bool) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	meta, err := readSessionMeta(dir)
	if err != nil || meta.Status.State == SessionExited {
		return "", false
	}
	meta.Run++
	if err := writeSessionMeta(dir, meta); err != nil {
		s.log.Warn("handoff: save meta", "session", id, "err", err)
		return "", false
	}
	return meta.ConversationID, true
}

// restartOnWorker ends the old conversation like an agent exit, records a new
// one and launches it under the worker tier. The caller holds metaMu.
func (s *Server) restartOnWorker(dir string, meta *sessionMeta, old string) error {
	s.saveConversationTotals(*meta, old)
	conv, err := nativerunner.NewUUID()
	if err != nil {
		return err
	}
	meta.ConversationID = conv
	s.applyTier(meta, ralphloop.TierWorker)
	meta.HandoffTier = true
	meta.FirstPrompt = "/gx-design " + ticketEpic(meta.Ticket)
	meta.Actionable = SessionActionable{}
	s.recordConversation(*meta)
	if err := clearOldHolder(dir); err != nil {
		return err
	}
	meta.Run++
	meta.Status, meta.StatusChangedAt, meta.Exit, meta.Stopped = SessionStatus{}, time.Now(), "", false
	if err := s.startSessionHolder(dir, meta, 0, 0, false); err != nil {
		return err
	}
	id, run := meta.ID, meta.Run
	s.sessionWatches.start(func() { s.watchSession(dir, id, run, time.Time{}, false) })
	s.events.publish(EventSessionChanged, id)
	return nil
}
