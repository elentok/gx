package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
)

// backupMaxAge is how long a backup ref lives; the next backup prunes older ones.
const backupMaxAge = 30 * 24 * time.Hour

// landOverDirtyTrunk fast-forwards target over the clashing uncommitted files
// in its checkout dir when putting them back afterwards would be clean: it
// backs them up to a gx ref, sets them aside, fast-forwards and restores them.
// landed is false, with nothing touched, when the restore would conflict.
// The caller holds the target's trunk lock.
func (s *Server) landOverDirtyTrunk(project string, repo git.Repo, dir, epic, target string, clashing []string) (landed bool, err error) {
	snapshot, err := git.SnapshotPaths(dir, clashing)
	if err != nil {
		return false, err
	}
	restored, clean, err := git.MergeSnapshotMarked(dir, snapshot, "refs/heads/"+epic)
	if err != nil || !clean {
		return false, err
	}
	ref, now, landed, err := s.landWithRestore(repo.Root, dir, epic, target, snapshot, restored, clashing,
		"restore after failed land", "landed")
	if err != nil {
		return landed, err
	}
	if pdir, err := s.projectDir(project); err == nil {
		_ = ralphloop.AppendEvent(pdir, epic, ralphloop.Event{
			Time: now, Type: string(events.DirtyTrunkLanded), Reason: strings.Join(clashing, ", "), BackupRef: ref,
		})
	}
	return true, nil
}

// landWithRestore fast-forwards target's checkout dir to epic over files, whose
// uncommitted content snapshot holds, and puts merged back in their place. The
// backup ref comes first: whatever happens to the files, they stay in git. When
// the fast-forward fails the files go back as they were and landed is false;
// landed is true when only the final restore failed. failLog and doneWord are
// the Warn message and the error wording of the two callers.
func (s *Server) landWithRestore(repoRoot, dir, epic, target, snapshot, merged string, files []string, failLog, doneWord string) (ref string, now time.Time, landed bool, err error) {
	ref, now, err = s.backupSnapshot(repoRoot, epic, snapshot)
	if err != nil {
		return "", now, false, err
	}
	if err := git.SetAside(dir, files); err != nil {
		return ref, now, false, fmt.Errorf("set aside %s: %w (backup %s)", listFiles(files), err, ref)
	}
	ok, err := git.MergeFastForward(dir, epic)
	if err != nil || !ok {
		if rerr := git.RestoreFromTree(dir, snapshot, files); rerr != nil {
			s.log.Warn(failLog, "epic", epic, "backup", ref, "err", rerr)
		}
		if err == nil {
			err = fmt.Errorf("%s is no longer a fast-forward of %s", epic, target)
		}
		return ref, now, false, err
	}
	if err := git.RestoreFromTree(dir, merged, files); err != nil {
		return ref, now, true, fmt.Errorf("%s, but restoring %s failed: %w (backup %s)", doneWord, listFiles(files), err, ref)
	}
	return ref, now, true, nil
}

// backupSnapshot names the snapshot commit with a new backup ref and prunes the
// old ones.
func (s *Server) backupSnapshot(repoRoot, epic, snapshot string) (ref string, now time.Time, err error) {
	now = time.Now()
	ref = fmt.Sprintf("%s%s-%d", git.BackupRefPrefix, epic, now.UnixNano())
	if err := git.UpdateRef(repoRoot, ref, snapshot); err != nil {
		return "", now, err
	}
	s.pruneBackups(repoRoot, now)
	return ref, now, nil
}

// pruneBackups drops the gx backup refs older than backupMaxAge. A failure is
// logged, never fatal: it must not stop a land.
func (s *Server) pruneBackups(repoRoot string, now time.Time) {
	refs, err := git.ListBackupRefs(repoRoot)
	if err != nil {
		s.log.Warn("list backup refs", "err", err)
		return
	}
	for _, r := range refs {
		if now.Sub(r.Time) <= backupMaxAge || s.backupInUse(r.Name) {
			continue
		}
		if err := git.DeleteRef(repoRoot, r.Name); err != nil {
			s.log.Warn("prune backup ref", "ref", r.Name, "err", err)
		}
	}
}
