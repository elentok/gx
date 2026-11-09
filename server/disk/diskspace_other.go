//go:build !darwin

package disk

import (
	"strconv"

	"github.com/elentok/gx/server/wire"
	"golang.org/x/sys/unix"
)

// readFreeSpace reads path's volume with one statfs and one stat. Linux has no
// mount point in Statfs_t, so the name is the device id.
func readFreeSpace(path string) (wire.DiskReading, error) {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return wire.DiskReading{}, err
	}
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return wire.DiskReading{}, err
	}
	id := strconv.FormatUint(uint64(st.Dev), 10)
	bsize := uint64(fs.Bsize)
	return wire.DiskReading{ID: id, Name: "device " + id, Free: uint64(fs.Bavail) * bsize, Total: fs.Blocks * bsize}, nil
}
