package holder

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// maxSocketPath is the longest unix socket path we bind directly. macOS caps
// sun_path at 104 bytes including the NUL; Linux allows 108. Staying under
// both leaves room for the terminator.
const maxSocketPath = 100

// aliasRoot holds the short socket paths of agent dirs that are too deep to
// bind in place. /tmp, not $TMPDIR: on macOS $TMPDIR alone is ~49 bytes.
const aliasRoot = "/tmp"

// SocketPath is where the holder serving dir listens. That is dir/holder.sock
// when the path fits the OS limit, so existing holders keep their address.
// Otherwise it is a short path derived only from dir, so the holder, a
// restarted server and the CLI all compute the same one without extra state.
func SocketPath(dir string) string {
	direct := filepath.Join(dir, HolderSocketFile)
	if len(direct) <= maxSocketPath {
		return direct
	}
	return filepath.Join(aliasDir(), fmt.Sprintf("%x", sha1.Sum([]byte(direct)))[:12]+".sock")
}

func aliasDir() string { return filepath.Join(aliasRoot, fmt.Sprintf("gx-%d", os.Getuid())) }

// prepareSocketDir creates the private directory a long path's alias lives in.
// It refuses one another user owns, since /tmp is shared.
func prepareSocketDir(sock string) error {
	dir := filepath.Dir(sock)
	if dir != aliasDir() {
		return nil // The socket sits in the agent dir, which exists.
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.IsDir() || !ok || int(st.Uid) != os.Getuid() || fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("socket dir %s is not a private directory owned by this user", dir)
	}
	return nil
}

// RemoveSocket removes dir's holder socket, wherever SocketPath put it, and
// the file in dir itself when a holder of another version left one there.
func RemoveSocket(dir string) error {
	for _, p := range []string{filepath.Join(dir, HolderSocketFile), SocketPath(dir)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
