// Package fsutil holds small filesystem helpers shared across packages. It is
// a leaf package so any package can import it without an import cycle.
package fsutil

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic replaces path's content via a same-directory temp file plus
// rename, so a concurrent reader always sees either the old or the new content
// in full, never a torn/truncated write from an in-place os.WriteFile.
func WriteFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
