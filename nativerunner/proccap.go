package nativerunner

import (
	"fmt"
	"log/slog"

	"github.com/elentok/gx/syshealth"
)

// ProcCap is the process limit for an agent launched now: the current process
// count plus headroom. The cap is fixed at launch, so a fork storm fails with
// EAGAIN instead of spreading. A count that cannot be read gives no cap (0)
// and the reason is logged: a launch never fails over it.
func ProcCap(count func() (int, error), headroom int, log *slog.Logger) int {
	if headroom <= 0 {
		// Unset settings, not a request for a cap at the current count.
		return 0
	}
	n, err := count()
	if err != nil {
		if log != nil {
			log.Warn("launching uncapped: cannot count processes", "err", err)
		}
		return 0
	}
	return n + headroom
}

// LiveProcCap is ProcCap with the headroom read at call time, so a settings
// change reaches the next launch. A nil headroom gets no cap; a nil count gets
// the default counter.
func LiveProcCap(headroom func() int, count func() (int, error), log *slog.Logger) int {
	if headroom == nil {
		return 0
	}
	if count == nil {
		count = DefaultProcCount
	}
	return ProcCap(count, headroom(), log)
}

// UlimitCommand is the shell command that applies the cap to the shell
// running it and to everything that shell starts afterwards.
func UlimitCommand(limit int) string {
	return fmt.Sprintf("ulimit -u %d", limit)
}

// CappedCommand wraps a launch so the program starts under the cap with no
// shell left in between: sh applies the cap, then execs the program, which
// keeps its own argv.
func CappedCommand(limit int, name string, args []string) (string, []string) {
	if limit <= 0 {
		return name, args
	}
	return "/bin/sh", append([]string{"-c", UlimitCommand(limit) + ` && exec "$@"`, "sh", name}, args...)
}

// DefaultProcCount counts every process the OS reports. That is at least the
// uid's own count, so the cap is never tighter than the uid's real usage.
func DefaultProcCount() (int, error) {
	t, err := syshealth.DefaultProcess().Read()
	if err != nil {
		return 0, err
	}
	return len(t.Processes), nil
}

// ProcCapArgs is the holder flag that carries the cap; none for no cap.
func ProcCapArgs(limit int) []string {
	if limit <= 0 {
		return nil
	}
	return []string{"--proc-cap", fmt.Sprint(limit)}
}
