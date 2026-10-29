package server

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// resumeSpacing staggers a server start's auto-resumes.
const resumeSpacing = 100 * time.Millisecond

// resumeCrashWindow is how soon an auto-resumed child may exit and still
// count as crashed, whatever its exit code.
const resumeCrashWindow = 10 * time.Second

// resumeSession runs the harness's Resume of an Exited session's newest
// conversation in a new holder, in the same dir. The gx session carries on:
// id, name, Unread, Actionable and Session ticket are kept.
func (s *Server) resumeSession(req SessionRequest) (SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State != SessionExited {
		return sessionRefusal(ReasonSessionLive, fmt.Sprintf("session %s is live", req.ID)), nil
	}
	if res := resumeRefusal(meta); res.Refused {
		return res, nil
	}
	meta.RestoredAt = time.Now()
	if meta.HandoffTier && meta.Exit == SessionExitCrashed {
		s.applyTier(&meta, meta.Tier)
	}
	if err := s.startResumedHolder(dir, &meta, time.Time{}); err != nil {
		s.endFailedStart(dir, &meta)
		return SessionResult{}, err
	}
	info := meta.info()
	return SessionResult{Session: &info}, nil
}

// resumeRefusal is why meta's session cannot be resumed, if it cannot.
func resumeRefusal(meta sessionMeta) SessionResult {
	if fi, err := os.Stat(meta.Dir); err != nil || !fi.IsDir() {
		return sessionRefusal(ReasonSessionDirGone, "dir is gone")
	}
	h, err := harness.Lookup(agentrunner.Kind(meta.Harness))
	if err != nil {
		return sessionRefusal(ReasonSessionUnknownHarness, err.Error())
	}
	if _, ok := h.Resume(meta.ConversationID, harness.LaunchOptions{}); !ok {
		return sessionRefusal(ReasonSessionNoResume, fmt.Sprintf("harness %s cannot resume", meta.Harness))
	}
	return SessionResult{}
}

// startResumedHolder starts the session's next run on its saved
// conversation and watches it; an exit before crashUntil is crashed. The
// caller holds metaMu.
func (s *Server) startResumedHolder(dir string, meta *sessionMeta, crashUntil time.Time) error {
	if err := clearOldHolder(dir); err != nil {
		return err
	}
	meta.Run++
	meta.Status, meta.StatusChangedAt, meta.Exit, meta.Stopped = SessionStatus{}, time.Now(), "", false
	meta.RestoreCrashed = false
	if err := s.startSessionHolder(dir, meta, 0, 0, true); err != nil {
		return err
	}
	id, run := meta.ID, meta.Run
	s.sessionWatches.start(func() { s.watchSession(dir, id, run, crashUntil, false) })
	s.events.publish(EventSessionChanged, id)
	return nil
}

// clearOldHolder waits for the old holder to stop serving, since closing its
// listener unlinks holder.sock by path and would take the new holder's. Then
// it drops what the old holder left: its exit code would read as the new
// holder's, and a killed holder leaves its socket file behind.
func clearOldHolder(dir string) error {
	deadline := time.Now().Add(sessionStopTimeout)
	for {
		if _, err := nativerunner.DialHolder(dir); err != nil {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("old holder still serving")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, f := range []string{nativerunner.ExitCodeFile, nativerunner.HolderSocketFile} {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// restoreSessions picks up every live session on server start. A live holder
// is watched again; the watch's first status, diffed against the saved one,
// raises any Unread missed while the server was down. A holder that left an
// exit, or of a session the person stopped, ended its session. Any other dead
// holder is auto-resumed once.
func (s *Server) restoreSessions() {
	list, err := s.listSessions()
	if err != nil {
		s.log.Warn("restore sessions", "err", err)
		return
	}
	var dead []string
	for _, info := range list {
		if info.Status.State == SessionExited {
			continue
		}
		dir := filepath.Join(s.cfg.SessionsDir, info.ID)
		if _, err := nativerunner.DialHolder(dir); err == nil {
			meta, err := readSessionMeta(dir)
			if err != nil {
				s.log.Warn("read session meta", "session", info.ID, "err", err)
				continue
			}
			s.sessionWatches.start(func() { s.watchSession(dir, info.ID, meta.Run, time.Time{}, true) })
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, nativerunner.ExitCodeFile)); err == nil || info.Stopped {
			exit := savedSessionExit(dir)
			s.updateSessionMetaOrWarn(dir, info.ID, func(m *sessionMeta) bool { return s.recordSessionExit(m, exit) })
			continue
		}
		dead = append(dead, info.ID)
	}
	s.sessionWatches.start(func() {
		for i, id := range dead {
			if i > 0 {
				select {
				case <-s.sessionWatches.stop:
					return
				case <-time.After(resumeSpacing):
				}
			}
			s.autoResumeSession(id)
		}
	})
}

// autoResumeSession resumes a session whose holder died while it was live,
// or ends it Exited crashed when auto-resume is off or cannot run. A session
// that was Working comes back with a "resumed" Unread; gx types nothing.
func (s *Server) autoResumeSession(id string) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(id)
	if res.Refused || err != nil {
		s.log.Warn("auto-resume session", "session", id, "err", err, "refusal", res.Message)
		return
	}
	if meta.Status.State == SessionExited || meta.Stopped {
		return
	}
	reason := "auto-resume is off"
	if s.settings.Load().AgentsResumeOnStart {
		reason = resumeRefusal(meta).Message
	}
	if reason == "" {
		ended := meta
		meta.RestoredAt = time.Time{}
		if meta.Status.State == SessionWorking {
			s.raiseUnread(&meta)
			meta.Unread.Resumed = meta.Unread.Unread
		}
		err := s.startResumedHolder(dir, &meta, time.Now().Add(resumeCrashWindow))
		if err == nil {
			return
		}
		meta, reason = ended, err.Error()
	}
	s.log.Info("session not auto-resumed", "session", id, "reason", reason)
	s.endFailedStart(dir, &meta)
}
