//go:build !darwin

package server

import (
	"strconv"

	"golang.org/x/sys/unix"
)

// readFreeSpace reads path's volume with one statfs and one stat. Linux has no
// mount point in Statfs_t, so the name is the device id.
func readFreeSpace(path string) (DiskReading, error) {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return DiskReading{}, err
	}
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return DiskReading{}, err
	}
	id := strconv.FormatUint(uint64(st.Dev), 10)
	return DiskReading{ID: id, Name: "device " + id, Free: uint64(fs.Bavail) * uint64(fs.Bsize)}, nil
}
