package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
)

const (
	fileDiskID   = "file"
	fileDiskName = "fixture disk"
	// fileDiskTotal is the size of the file-backed disk when the file names no total.
	fileDiskTotal = 500 * 1000 * 1000 * 1000
)

// FileFreeSpace is a Config.FreeSpace source for tests that drive the watcher
// from outside the process. Every path reads the one disk whose free space is
// the size string in the file at free: "8G" is 8G free of a fixed 500G, "8G/500G"
// names the total. An empty or unreadable file is a failed read.
func FileFreeSpace(free string) func(path string) (wire.DiskReading, error) {
	return func(string) (wire.DiskReading, error) {
		b, err := os.ReadFile(free)
		if err != nil {
			return wire.DiskReading{}, err
		}
		s := strings.TrimSpace(string(b))
		if s == "" {
			return wire.DiskReading{}, fmt.Errorf("%s is empty", free)
		}
		freeStr, totalStr, hasTotal := strings.Cut(s, "/")
		n, err := config.ParseByteSize(strings.TrimSpace(freeStr))
		if err != nil {
			return wire.DiskReading{}, fmt.Errorf("%s: %w", free, err)
		}
		total := config.ByteSize(fileDiskTotal)
		if hasTotal {
			if total, err = config.ParseByteSize(strings.TrimSpace(totalStr)); err != nil {
				return wire.DiskReading{}, fmt.Errorf("%s: %w", free, err)
			}
		}
		return wire.DiskReading{ID: fileDiskID, Name: fileDiskName, Free: uint64(n), Total: uint64(total)}, nil
	}
}
