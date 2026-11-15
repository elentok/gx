package skills

import (
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestSharedRefresh_OverlappingCallsShareOneRun(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	runs := 0
	f := NewSharedRefresh(func() error {
		runs++
		started <- struct{}{}
		<-release
		return errors.New("shared")
	})
	errs := make(chan error, 2)
	go func() { errs <- f.Refresh() }()
	<-started
	go func() { errs <- f.Refresh() }()
	deadline := time.Now().Add(5 * time.Second)
	for f.waiting.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("second call never joined the run")
		}
		runtime.Gosched()
	}
	close(release)
	for range 2 {
		if err := <-errs; err == nil || err.Error() != "shared" {
			t.Errorf("err = %v, want the shared result", err)
		}
	}
	if runs != 1 {
		t.Errorf("runs = %d, want 1", runs)
	}
}
