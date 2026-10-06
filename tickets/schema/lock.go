package schema

import (
	"fmt"
	"os"
	"time"
)

const ticketLockTimeout = 10 * time.Second

// LockTicket takes an exclusive per-ticket lock (a sidecar "<path>.lock" file,
// same O_EXCL scheme as tickets.LockEpic) and returns its unlock func. Every
// read-modify-write of a ticket file must hold it, so a fork parent's
// full-content write can't land over its child's concurrent `claimed`.
func LockTicket(path string) (unlock func(), err error) {
	lockPath := path + ".lock"
	deadline := time.Now().Add(ticketLockTimeout)

	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			f.Close()
			return func() { os.Remove(lockPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquiring ticket lock at %s: %w", lockPath, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for ticket lock at %s", lockPath)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
