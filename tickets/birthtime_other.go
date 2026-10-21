//go:build !darwin

package tickets

import (
	"os"
	"time"
)

// fileCreationTime is the file's mtime: only darwin reports a creation time
// through os.FileInfo.
func fileCreationTime(info os.FileInfo) time.Time {
	return info.ModTime().UTC()
}
