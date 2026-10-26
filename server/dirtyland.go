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
	restored, clean, err := git.MergeSnapshotOnto(dir, snapshot, "refs/heads/"+epic)
	if err != nil || !clean {
		return false, err
	}
	// The backup comes first: whatever happens to the files, they stay in git.
	ref, now, err := s.backupSnapshot(repo.Root, epic, snapshot)
	if err != nil {
		return false, err
	}
	if err := git.SetAside(dir, clashing); err != nil {
		return false, fmt.Errorf("set aside %s: %w (backup %s)", listFiles(clashing), err, ref)
	}
	ok, err := git.MergeFastForward(dir, epic)
	if err != nil || !ok {
		// Not landed: put the files back as they were.
		if rerr := git.RestoreFromTree(dir, snapshot, clashing); rerr != nil {
			s.log.Warn("restore after failed land", "epic", epic, "backup", ref, "err", rerr)
		}
		if err == nil {
			err = fmt.Errorf("%s is no longer a fast-forward of %s", epic, target)
		}
		return false, err
	}
	if err := git.RestoreFromTree(dir, restored, clashing); err != nil {
		return true, fmt.Errorf("landed, but restoring %s failed: %w (backup %s)", listFiles(clashing), err, ref)
	}
	if pdir, err := s.projectDir(project); err == nil {
		_ = ralphloop.AppendEvent(pdir, epic, ralphloop.Event{
			Time: now, Type: string(events.DirtyTrunkLanded), Reason: strings.Join(clashing, ", "), BackupRef: ref,
		})
	}
	return true, nil
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
