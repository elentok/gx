package ralphloop

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// realGitTestTimeout bounds how long a single real-git-backed test may run
// before realGitTimeoutWatchdog fails it by name, well under the package
// -timeout (ticket 02) that would otherwise be the only signal something
// hung — and only after killing every test in the package and dumping every
// goroutine stack. Comfortably above the slowest legitimate real-git test's
// runtime (a few seconds) to avoid flaking healthy tests.
const realGitTestTimeout = 30 * time.Second

// timeoutReporter is the subset of *testing.T realGitTimeoutWatchdog needs.
// Letting callers substitute a fake satisfies TestRealGitTimeoutWatchdog_*
// below without requiring an actual multi-second hang under `go test`.
type timeoutReporter interface {
	Helper()
	Cleanup(func())
	Fatalf(format string, args ...any)
	Name() string
}

// realGitTimeoutWatchdog fails t by name if it hasn't finished (its Cleanup
// hasn't run) within bound, so a hung real-git test is pinpointed directly
// instead of waiting for the package's own -timeout. Call as the first line
// of a real-git-backed test (or of each t.Run subtest, for table-driven
// tests).
func realGitTimeoutWatchdog(t timeoutReporter, bound time.Duration) {
	t.Helper()
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })

	timer := time.NewTimer(bound)
	go func() {
		select {
		case <-done:
			timer.Stop()
		case <-timer.C:
			t.Fatalf("%s exceeded its %s bound: likely hung; failing it directly instead of waiting for the package timeout", t.Name(), bound)
		}
	}()
}

// fakeTimeoutReporter is a timeoutReporter that records registered cleanups
// (run only when the test explicitly calls finish, mirroring *testing.T
// running them when the real test function returns) and Fatalf calls on a
// channel, so the fixture tests below can prove realGitTimeoutWatchdog fires
// (or stays silent) without driving a real *testing.T past its bound.
type fakeTimeoutReporter struct {
	name string

	mu       sync.Mutex
	cleanups []func()
	fataled  chan string
}

func newFakeTimeoutReporter(name string) *fakeTimeoutReporter {
	return &fakeTimeoutReporter{name: name, fataled: make(chan string, 1)}
}

func (f *fakeTimeoutReporter) Helper()      {}
func (f *fakeTimeoutReporter) Name() string { return f.name }

func (f *fakeTimeoutReporter) Cleanup(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, fn)
}

func (f *fakeTimeoutReporter) Fatalf(format string, args ...any) {
	select {
	case f.fataled <- fmt.Sprintf(format, args...):
	default:
	}
}

// finish runs every registered cleanup, simulating the wrapped test's Run()
// returning.
func (f *fakeTimeoutReporter) finish() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, fn := range f.cleanups {
		fn()
	}
}

// TestRealGitTimeoutWatchdog_FiresOnHang proves acceptance criterion 3: a
// test that blocks past its configured bound (never calls finish, simulating
// Run() never returning) is failed fast, with a message identifying it by
// name, well before any package-level timeout would fire.
func TestRealGitTimeoutWatchdog_FiresOnHang(t *testing.T) {
	t.Parallel()
	fake := newFakeTimeoutReporter("TestFakeHungRealGitTest")
	realGitTimeoutWatchdog(fake, 20*time.Millisecond)

	select {
	case msg := <-fake.fataled:
		if !strings.Contains(msg, fake.name) || !strings.Contains(msg, "exceeded its") {
			t.Errorf("watchdog message = %q, want it to name %q and explain the bound was exceeded", msg, fake.name)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog did not fire within 1s for a 20ms bound")
	}
}

// TestRealGitTimeoutWatchdog_NoFireWhenFinishedInTime proves the watchdog
// stays silent for a test that finishes before the bound, so it can't flake
// a healthy test.
func TestRealGitTimeoutWatchdog_NoFireWhenFinishedInTime(t *testing.T) {
	t.Parallel()
	fake := newFakeTimeoutReporter("TestFakeHealthyRealGitTest")
	realGitTimeoutWatchdog(fake, 200*time.Millisecond)
	fake.finish()
	time.Sleep(400 * time.Millisecond)

	select {
	case msg := <-fake.fataled:
		t.Errorf("watchdog fired for a test that finished in time: %q", msg)
	default:
	}
}
