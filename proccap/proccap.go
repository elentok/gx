// Package proccap computes the process cap (`ulimit -u` headroom) an agent
// launches under and builds the commands that apply it. The runners, the
// session manager and runnerbuild share it, so none depends on another's
// runner package for it.
package proccap

import (
	"fmt"
	"log/slog"

	"github.com/elentok/gx/syshealth"
)

// Cap is the process limit for an agent launched now: the current process
// count plus headroom. The cap is fixed at launch, so a fork storm fails with
// EAGAIN instead of spreading. A count that cannot be read gives no cap (0)
// and the reason is logged: a launch never fails over it.
func Cap(count func() (int, error), headroom int, log *slog.Logger) int {
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

// Source is where a launch reads its cap from. It is the one place the
// counter can be injected.
type Source struct {
	// Headroom is read at each launch, so a settings change reaches the next
	// one. Nil means no cap.
	Headroom func() int
	// Count counts processes; nil means DefaultCount. Tests inject it.
	Count func() (int, error)
}

// Limit is the cap for a launch now; zero is none.
func (s Source) Limit(log *slog.Logger) int {
	if s.Headroom == nil {
		return 0
	}
	count := s.Count
	if count == nil {
		count = DefaultCount
	}
	return Cap(count, s.Headroom(), log)
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

// DefaultCount counts every process the OS reports. That is at least the
// uid's own count, so the cap is never tighter than the uid's real usage.
func DefaultCount() (int, error) {
	t, err := syshealth.DefaultProcess().Read()
	if err != nil {
		return 0, err
	}
	return len(t.Processes), nil
}

// Args is the holder flag that carries the cap; none for no cap.
func Args(limit int) []string {
	if limit <= 0 {
		return nil
	}
	return []string{"--proc-cap", fmt.Sprint(limit)}
}
