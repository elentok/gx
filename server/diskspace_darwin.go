//go:build darwin

package server

import (
	"strconv"

	"golang.org/x/sys/unix"
)

// readFreeSpace reads path's volume with one statfs and one stat.
func readFreeSpace(path string) (DiskReading, error) {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return DiskReading{}, err
	}
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return DiskReading{}, err
	}
	return DiskReading{
		ID:    strconv.FormatUint(uint64(st.Dev), 10),
		Name:  unix.ByteSliceToString(fs.Mntonname[:]),
		Free:  fs.Bavail * uint64(fs.Bsize),
		Total: fs.Blocks * uint64(fs.Bsize),
	}, nil
}
