package sessions

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
)

// resumeSpacing staggers a server start's auto-resumes.
const resumeSpacing = 100 * time.Millisecond

// resumeCrashWindow is how soon an auto-resumed child may exit and still
// count as crashed, whatever its exit code.
const resumeCrashWindow = 10 * time.Second

// ResumeSession runs the harness's Resume of an Exited session's newest
// conversation in a new holder, in the same dir. The gx session carries on:
// id, name, Unread, Actionable and Session ticket are kept.
func (rs *Resumer) ResumeSession(req wire.SessionRequest) (wire.SessionResult, error) {
	rs.mg.Watches.MetaMu.Lock()
	defer rs.mg.Watches.MetaMu.Unlock()
	dir, meta, res, err := rs.mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State != wire.SessionExited {
		return Refusal(wire.ReasonSessionLive, fmt.Sprintf("session %s is live", req.ID)), nil
	}
	if res := rs.resumeRefusal(meta); res.Refused {
		return res, nil
	}
	meta.RestoredAt = time.Now()
	if meta.HandoffTier && meta.Exit == wire.SessionExitCrashed {
		rs.mg.applyTier(&meta, meta.Tier)
	}
	if err := rs.startResumedHolder(dir, &meta, time.Time{}); err != nil {
		rs.mg.endFailedStart(dir, &meta)
		return wire.SessionResult{}, err
	}
	info := meta.Info()
	return wire.SessionResult{Session: &info}, nil
}

// resumeRefusal is why meta's session cannot be resumed, if it cannot.
func (rs *Resumer) resumeRefusal(meta Meta) wire.SessionResult {
	if fi, err := os.Stat(meta.Dir); err != nil || !fi.IsDir() {
		return Refusal(wire.ReasonSessionDirGone, "dir is gone")
	}
	h, err := harness.Lookup(agentrunner.Kind(meta.Harness))
	if err != nil {
		return Refusal(wire.ReasonSessionUnknownHarness, err.Error())
	}
	if _, ok := h.Resume(meta.ConversationID, harness.LaunchOptions{}); !ok {
		return Refusal(wire.ReasonSessionNoResume, fmt.Sprintf("harness %s cannot resume", meta.Harness))
	}
	if agentrunner.Kind(meta.Harness) == harness.Codex && !rs.mg.rollouts.CodexRolloutExists(meta.Dir, meta.ConversationID) {
		return Refusal(wire.ReasonSessionNoResume, fmt.Sprintf("no Codex rollout for conversation %q in %s", meta.ConversationID, meta.Dir))
	}
	if msg := rs.mg.harness.InteractiveMissingReason(meta.Harness, "the session's harness"); msg != "" {
		return Refusal(wire.ReasonSessionHarnessMissing, msg)
	}
	return wire.SessionResult{}
}

// startResumedHolder starts the session's next run on its saved
// conversation and watches it; an exit before crashUntil is crashed. The
// caller holds metaMu.
func (rs *Resumer) startResumedHolder(dir string, meta *Meta, crashUntil time.Time) error {
	if err := clearOldHolder(dir); err != nil {
		return err
	}
	meta.Run++
	meta.Status, meta.StatusChangedAt, meta.Exit, meta.Stopped = wire.SessionStatus{}, time.Now(), "", false
	meta.RestoreCrashed = false
	if err := rs.mg.startSessionHolder(dir, meta, 0, 0, true); err != nil {
		return err
	}
	id, run := meta.ID, meta.Run
	rs.mg.Watches.Start(func() { rs.mg.watchSession(dir, id, run, crashUntil, false) })
	rs.mg.events.Publish(wire.EventSessionChanged, id)
	return nil
}

// clearOldHolder waits for the old holder to stop serving, since closing its
// listener unlinks holder.sock by path and would take the new holder's. Then
// it drops what the old holder left: its exit code would read as the new
// holder's, and a killed holder leaves its socket file behind.
func clearOldHolder(dir string) error {
	deadline := time.Now().Add(sessionStopTimeout)
	for {
		if _, err := holder.DialHolder(dir); err != nil {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("old holder still serving")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, f := range []string{holder.ExitCodeFile, holder.HolderSocketFile} {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// RestoreSessions picks up every live session on server start. A live holder
// is watched again; the watch's first status, diffed against the saved one,
// raises any Unread missed while the server was down. A holder that left an
// exit, or of a session the person stopped, ended its session. Any other dead
// holder is auto-resumed once.
func (rs *Resumer) RestoreSessions() {
	list, err := rs.mg.ListSessions()
	if err != nil {
		rs.mg.log.Warn("restore sessions", "err", err)
		return
	}
	var dead []string
	for _, info := range list {
		if info.Status.State == wire.SessionExited {
			continue
		}
		dir := filepath.Join(rs.mg.cfg.SessionsDir, info.ID)
		if _, err := holder.DialHolder(dir); err == nil {
			meta, err := ReadMeta(dir)
			if err != nil {
				rs.mg.log.Warn("read session meta", "session", info.ID, "err", err)
				continue
			}
			// The probe's memory of the launch override died with the old server.
			if meta.Harness == string(harness.Claude) {
				harness.NoteEditorOverride(meta.ConversationID, harness.LaunchEnv(os.Environ(), rs.mg.cfg.Env))
			}
			rs.mg.Watches.Start(func() { rs.mg.watchSession(dir, info.ID, meta.Run, time.Time{}, true) })
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, holder.ExitCodeFile)); err == nil || info.Stopped {
			exit := savedSessionExit(dir)
			rs.mg.UpdateSessionMetaOrWarn(dir, info.ID, func(m *Meta) bool { return rs.mg.recordSessionExit(m, exit) })
			continue
		}
		dead = append(dead, info.ID)
	}
	rs.mg.Watches.Start(func() {
		for i, id := range dead {
			if i > 0 {
				select {
				case <-rs.mg.Watches.Stop:
					return
				case <-time.After(resumeSpacing):
				}
			}
			rs.autoResumeSession(id)
		}
	})
}

// autoResumeSession resumes a session whose holder died while it was live,
// or ends it Exited crashed when auto-resume is off or cannot run. A session
// that was Working comes back with a "resumed" Unread; gx types nothing.
func (rs *Resumer) autoResumeSession(id string) {
	rs.mg.Watches.MetaMu.Lock()
	defer rs.mg.Watches.MetaMu.Unlock()
	dir, meta, res, err := rs.mg.SessionDir(id)
	if res.Refused || err != nil {
		rs.mg.log.Warn("auto-resume session", "session", id, "err", err, "refusal", res.Message)
		return
	}
	if meta.Status.State == wire.SessionExited || meta.Stopped {
		return
	}
	reason := "auto-resume is off"
	if rs.mg.settings.Load().AgentsResumeOnStart {
		reason = rs.resumeRefusal(meta).Message
	}
	if reason == "" {
		ended := meta
		meta.RestoredAt = time.Time{}
		if meta.Status.State == wire.SessionWorking {
			rs.mg.raiseUnread(&meta)
			meta.Unread.Resumed = meta.Unread.Unread
		}
		err := rs.startResumedHolder(dir, &meta, time.Now().Add(resumeCrashWindow))
		if err == nil {
			return
		}
		meta, reason = ended, err.Error()
	}
	rs.mg.log.Info("session not auto-resumed", "session", id, "reason", reason)
	rs.mg.endFailedStart(dir, &meta)
}
