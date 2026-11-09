package holder

import "golang.org/x/sys/unix"

// ApplyProcCap lowers this process's RLIMIT_NPROC to limit; its children,
// the agent and everything it runs, inherit it. Zero or less leaves the limit
// alone.
func ApplyProcCap(limit int) error {
	if limit <= 0 {
		return nil
	}
	l := unix.Rlimit{Cur: uint64(limit), Max: uint64(limit)}
	return unix.Setrlimit(unix.RLIMIT_NPROC, &l)
}
