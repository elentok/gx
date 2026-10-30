package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/sys/unix"
)

// The lock file stays in place so competing processes always lock the same
// inode. The kernel releases the advisory lock when a process exits.
func lockInstallation(manifestPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(manifestPath+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}
	return func() { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN); _ = file.Close() }, nil
}

type savedPath struct {
	Path     string `json:"path"`
	Snapshot string `json:"snapshot,omitempty"`
}

// clonePath uses Lstat at every level. Backups retain directory links themselves,
// including broken links, rather than reading their targets.
func clonePath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}
	if info.IsDir() {
		if err := os.Mkdir(dst, 0700); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := clonePath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return os.Chmod(dst, info.Mode().Perm())
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("cannot back up special file %s", src)
	}
	bytes, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, bytes, info.Mode().Perm())
}

// transactionIO supplies failure seams for filesystem-level rollback tests,
// without process-global hooks or reading a developer's live installation.
type transactionIO struct {
	apply   func(operation) error
	restore func(savedPath) error
}

func applyOperation(op operation) error {
	if err := os.MkdirAll(filepath.Dir(op.path), 0755); err != nil {
		return err
	}
	if err := os.RemoveAll(op.path); err != nil {
		return err
	}
	if op.remove {
		return nil
	}
	if op.entry.Kind == KindFileLink || op.entry.Kind == KindDirectoryLink {
		return os.Symlink(op.entry.LinkTarget, op.path)
	}
	return os.WriteFile(op.path, op.data, 0644)
}

func restorePath(saved savedPath) error {
	if err := os.RemoveAll(saved.Path); err != nil {
		return err
	}
	if saved.Snapshot == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(saved.Path), 0755); err != nil {
		return err
	}
	return clonePath(saved.Snapshot, saved.Path)
}

func executePlan(p *installationPlan, manifestPath string, publish func() error) (string, error) {
	return executePlanWithIO(p, manifestPath, publish, transactionIO{apply: applyOperation, restore: restorePath})
}

func executePlanWithIO(p *installationPlan, manifestPath string, publish func() error, io transactionIO) (string, error) {
	// Snapshot disjoint topmost paths. A converted directory covers all of the
	// child writes that follow it. Include missing parents for exact rollback.
	paths := make([]string, 0, len(p.operations))
	for _, op := range p.operations {
		path := op.path
		for parent := filepath.Dir(path); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			_, err := os.Lstat(parent)
			if err == nil {
				break
			}
			if !os.IsNotExist(err) {
				return "", err
			}
			path = parent
		}
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) < len(paths[j])
		}
		return paths[i] < paths[j]
	})
	var saved []savedPath
	for _, path := range paths {
		covered := false
		for _, s := range saved {
			if within(path, s.Path) {
				covered = true
				break
			}
		}
		if !covered {
			saved = append(saved, savedPath{Path: path})
		}
	}
	backupRoot, err := physicalPath(p.backupDir)
	if err != nil {
		return "", err
	}
	for _, s := range saved {
		if within(backupRoot, s.Path) {
			return "", fmt.Errorf("backup directory %s is inside destination %s", backupRoot, s.Path)
		}
	}
	if err := os.MkdirAll(backupRoot, 0700); err != nil {
		return "", err
	}
	backup, err := os.MkdirTemp(backupRoot, time.Now().UTC().Format("20060102T150405Z-")+"*")
	if err != nil {
		return "", err
	}
	for i := range saved {
		if _, err := os.Lstat(saved[i].Path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return backup, err
		}
		saved[i].Snapshot = filepath.Join(backup, fmt.Sprintf("%04d", i))
		if err := clonePath(saved[i].Path, saved[i].Snapshot); err != nil {
			return backup, fmt.Errorf("backup %s: %w", saved[i].Path, err)
		}
	}
	// Keep the previous manifest with the recovery index. Publication is atomic,
	// so a failed publish still leaves the previous manifest in place.
	if _, err := os.Lstat(manifestPath); err == nil {
		if err := clonePath(manifestPath, filepath.Join(backup, "manifest.json")); err != nil {
			return backup, err
		}
	} else if !os.IsNotExist(err) {
		return backup, err
	}
	bytes, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return backup, err
	}
	if err := os.WriteFile(filepath.Join(backup, "paths.json"), bytes, 0600); err != nil {
		return backup, err
	}
	rollback := func(cause error) (string, error) {
		var failures []error
		for i := len(saved) - 1; i >= 0; i-- {
			if err := io.restore(saved[i]); err != nil {
				failures = append(failures, fmt.Errorf("restore %s: %w", saved[i].Path, err))
			}
		}
		if len(failures) > 0 {
			return backup, fmt.Errorf("%w; rollback failed: %v; recover from %s", cause, errors.Join(failures...), backup)
		}
		return backup, fmt.Errorf("%w; installation restored; backups: %s", cause, backup)
	}
	for _, op := range p.operations {
		if err := io.apply(op); err != nil {
			return rollback(fmt.Errorf("change %s: %w", op.path, err))
		}
	}
	if err := publish(); err != nil {
		return rollback(fmt.Errorf("publish manifest: %w", err))
	}
	return backup, nil
}
