package testutil

import (
	"io/fs"
	"os"
	"path/filepath"
)

// MakeWritable adds the owner write bit to every dir under root. Go marks its
// module cache read-only, which makes os.RemoveAll fail on a temp HOME that
// ran `go`. Errors are ignored: the RemoveAll that follows reports them.
func MakeWritable(root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			if info, err := d.Info(); err == nil {
				_ = os.Chmod(path, info.Mode().Perm()|0o700)
			}
		}
		return nil
	})
}
