package syshealth

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestParentGone_CallsOnGoneOnceTheOwnerDies(t *testing.T) {
	var dead atomic.Bool
	gone := make(chan struct{})
	stop := WatchOwner(42, time.Millisecond, func(pid int) bool {
		if pid != 42 {
			t.Errorf("polled pid %d, want 42", pid)
		}
		return !dead.Load()
	}, func() { close(gone) })
	defer stop()

	select {
	case <-gone:
		t.Fatal("onGone ran while the owner was alive")
	case <-time.After(20 * time.Millisecond):
	}
	dead.Store(true)
	select {
	case <-gone:
	case <-time.After(5 * time.Second):
		t.Fatal("onGone did not run after the owner died")
	}
}

func TestParentGone_UnsetOrMalformedEnvWatchesNothing(t *testing.T) {
	for _, v := range []string{"", "not-a-pid", "0"} {
		t.Setenv(OwnerPIDEnv, v)
		if WatchOwnerFromEnv(func() { t.Error("onGone ran") }) {
			t.Errorf("started a watch for %q", v)
		}
	}
}
