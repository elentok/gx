package fixturetest

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// acquireAsync starts an acquire and returns a channel that yields its release.
func acquireAsync(t *testing.T, dir string, slots int) <-chan func() {
	ch := make(chan func(), 1)
	go func() {
		release, err := acquireLaunchSlot(dir, slots)
		if err != nil {
			t.Error(err)
			return
		}
		ch <- release
	}()
	return ch
}

func assertWaits(t *testing.T, ch <-chan func()) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("acquire returned while every slot was held")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAcquireLaunchSlot_OneSlotBlocksUntilReleased(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireLaunchSlot(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	second := acquireAsync(t, dir, 1)
	assertWaits(t, second)
	first()
	select {
	case release := <-second:
		release()
	case <-time.After(5 * time.Second):
		t.Fatal("second acquire did not return after release")
	}
}

func TestAcquireLaunchSlot_TwoSlotsAdmitTwoAndWaitOnThird(t *testing.T) {
	dir := t.TempDir()
	a, err := acquireLaunchSlot(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := acquireLaunchSlot(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	third := acquireAsync(t, dir, 2)
	assertWaits(t, third)
	a()
	select {
	case release := <-third:
		release()
	case <-time.After(5 * time.Second):
		t.Fatal("third acquire did not return after release")
	}
	b()
}

func TestAcquireLaunchSlot_DoubleReleaseKeepsOthersSlot(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireLaunchSlot(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	first()
	second, err := acquireLaunchSlot(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	first() // must not free second's slot
	third := acquireAsync(t, dir, 1)
	assertWaits(t, third)
	second()
	select {
	case release := <-third:
		release()
	case <-time.After(5 * time.Second):
		t.Fatal("third acquire did not return after release")
	}
}

func TestExplainStartErr_NamesTheProcessLimit(t *testing.T) {
	err := explainStartErr(&os.PathError{Op: "fork/exec", Path: "gx", Err: syscall.EAGAIN})
	if err == nil || !strings.Contains(err.Error(), "process limit hit") {
		t.Fatalf("err = %v, want the process-limit message", err)
	}
	other := fmt.Errorf("boom")
	if got := explainStartErr(other); got != other {
		t.Errorf("a non-EAGAIN error was rewritten: %v", got)
	}
	if explainStartErr(nil) != nil {
		t.Error("nil became an error")
	}
}
