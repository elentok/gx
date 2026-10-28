package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/elentok/gx/config"
)

const (
	fileDiskID   = "file"
	fileDiskName = "fixture disk"
)

// FileFreeSpace is a Config.FreeSpace source for tests that drive the watcher
// from outside the process. Every path reads the one disk whose free space is
// the size string in the file at free. An empty or unreadable file is a failed read.
func FileFreeSpace(free string) func(path string) (DiskReading, error) {
	return func(string) (DiskReading, error) {
		b, err := os.ReadFile(free)
		if err != nil {
			return DiskReading{}, err
		}
		s := strings.TrimSpace(string(b))
		if s == "" {
			return DiskReading{}, fmt.Errorf("%s is empty", free)
		}
		n, err := config.ParseByteSize(s)
		if err != nil {
			return DiskReading{}, fmt.Errorf("%s: %w", free, err)
		}
		return DiskReading{ID: fileDiskID, Name: fileDiskName, Free: uint64(n)}, nil
	}
}
