package tickets

import (
	"os"
	"syscall"
	"time"
)

// fileCreationTime is when the file was created, falling back to its mtime
// when the platform doesn't report it.
func fileCreationTime(info os.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Birthtimespec.Sec, st.Birthtimespec.Nsec).UTC()
	}
	return info.ModTime().UTC()
}
