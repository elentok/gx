package server

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// tempSweepName matches the temp dirs gx's own tests and `go` leave behind when
// they are killed before their cleanup runs: gx-epic-* (a spinning test's run-log
// reached 2 GiB), go-build*, gxf* (a gxfixture root) and gxs* (a test server's
// state dir).
var tempSweepName = regexp.MustCompile(`^(gx-epic-|go-build|gxf|gxs)[0-9]+$`)

// defaultTempSweepMinAge is how long a dir must sit untouched before the sweep
// removes it; a constant, like defaultCleanTimeout.
const defaultTempSweepMinAge = time.Hour

// sweepStaleTemp removes the stale test dirs of dir (see tempSweepName). It
// skips a dir a running process names in its command line, and a dir with any
// entry changed within minAge, so a live test or build is never pulled out
// from under itself. It returns the dirs it removed.
func sweepStaleTemp(dir string, minAge time.Duration, inUse func() string, now time.Time) (removed []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	running := ""
	loaded := false
	for _, e := range entries {
		if !e.IsDir() || !tempSweepName.MatchString(e.Name()) {
			continue
		}
		if !loaded {
			running, loaded = inUse(), true
		}
		if strings.Contains(running, e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if changedWithin(path, now.Add(-minAge)) {
			continue
		}
		makeWritable(path)
		if os.RemoveAll(path) == nil {
			removed = append(removed, path)
		}
	}
	return removed
}

// makeWritable adds the owner write bit to every dir under root: Go marks its
// module cache read-only, which makes os.RemoveAll fail.
func makeWritable(root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			if info, err := d.Info(); err == nil {
				_ = os.Chmod(path, info.Mode().Perm()|0o700)
			}
		}
		return nil
	})
}

// changedWithin reports whether path, or anything under it, changed after cutoff.
// An unreadable entry counts as changed, so the sweep leaves it alone.
func changedWithin(path string, cutoff time.Time) bool {
	found := false
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			found = true
			return fs.SkipAll
		}
		if info, err := d.Info(); err != nil || info.ModTime().After(cutoff) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// processCommands lists every running process's command line, one per line.
// ps runs in env, which is the server's configured environment.
func processCommands(env []string) string {
	cmd := exec.Command("ps", "-axo", "command=")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// sweepTemp runs the sweep over the temp dirs and logs what it removed. The
// configured TempSweepDir stands alone; otherwise it covers os.TempDir() and
// /tmp, where the short-path test roots (gxf*, gxs*) go.
func (s *Server) sweepTemp() {
	dirs := []string{s.cfg.TempSweepDir}
	if s.cfg.TempSweepDir == "" {
		dirs = []string{os.TempDir(), "/tmp"}
	}
	minAge := s.cfg.TempSweepMinAge
	if minAge <= 0 {
		minAge = defaultTempSweepMinAge
	}
	seen := map[string]bool{}
	for _, dir := range dirs {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		if removed := sweepStaleTemp(dir, minAge, func() string { return processCommands(s.cfg.Env) }, time.Now()); len(removed) > 0 {
			s.log.Info("disk clean removed stale temp dirs", "count", len(removed), "dir", dir)
		}
	}
}
