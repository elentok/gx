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
	if fi, err := os.Stat(meta.Dir); err != nil || !fi.IsDir() {
		return sessionRefusal(ReasonSessionDirGone, "dir is gone"), nil
	}
	h, err := harness.Lookup(agentrunner.Kind(meta.Harness))
	if err != nil {
		return sessionRefusal(ReasonSessionUnknownHarness, err.Error()), nil
	}
	if _, ok := h.Resume(meta.ConversationID, harness.LaunchOptions{}); !ok {
		return sessionRefusal(ReasonSessionNoResume, fmt.Sprintf("harness %s cannot resume", meta.Harness)), nil
	}
	if err := clearOldHolder(dir); err != nil {
		return SessionResult{}, err
	}
	meta.Run++
	meta.Status, meta.StatusChangedAt, meta.Exit, meta.Stopped = SessionStatus{}, time.Now(), "", false
	if err := s.startSessionHolder(dir, &meta, 0, 0, true); err != nil {
		return SessionResult{}, err
	}
	run := meta.Run
	s.sessionWatches.start(func() { s.watchSession(dir, req.ID, run) })
	s.events.publish(EventSessionChanged, req.ID)
	info := meta.info()
	return SessionResult{Session: &info}, nil
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
