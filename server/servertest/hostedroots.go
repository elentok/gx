package servertest

import (
	"os"
	"path/filepath"
	"testing"
)

// OutsideTempDir makes a folder that is no hosted-file root: t.TempDir() lies
// under the temp roots, which gx hosts without any setting.
func OutsideTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/var/tmp", "gxroots")
	if err != nil {
		t.Skipf("no /var/tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		dir = r
	}
	return dir
}
