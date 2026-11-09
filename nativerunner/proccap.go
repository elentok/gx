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
