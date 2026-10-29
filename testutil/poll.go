package testutil

import (
	"testing"
	"time"
)

// Poll checks cond every `every` until it holds or timeout passes, and reports
// whether it held. It is the one condition wait for tests outside the server
// package (the sleep guard does not count this file). Pick a long `every` when
// cond is costly, such as one that runs a gx subprocess.
func Poll(timeout, every time.Duration, cond func() bool) bool {
	for deadline := time.Now().Add(timeout); !cond(); time.Sleep(every) {
		if time.Now().After(deadline) {
			return false
		}
	}
	return true
}

// WaitFor is Poll with a 10ms step that fails the test with "timed out
// waiting for <what>" when cond never holds.
func WaitFor(t testing.TB, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	if !Poll(timeout, 10*time.Millisecond, cond) {
		t.Fatalf("timed out waiting for %s", what)
	}
}
