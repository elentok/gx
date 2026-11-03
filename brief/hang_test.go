package brief

import (
	"testing"
	"time"
)

func feed(t *testing.T, h *hangWatcher, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if _, err := h.Write([]byte(l + "\n")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHangWatcher_FlagsATestRunningPastTheLimit(t *testing.T) {
	h := newHangWatcher(time.Minute)
	feed(t, h, `{"Action":"run","Package":"p","Test":"TestA"}`)
	if !h.check(time.Now().Add(2 * time.Minute)) {
		t.Fatal("a plain test past the limit was not flagged")
	}
}

func TestHangWatcher_IgnoresAParentWaitingForItsParallelSubtests(t *testing.T) {
	h := newHangWatcher(time.Minute)
	feed(t, h,
		`{"Action":"run","Package":"p","Test":"TestA"}`,
		`{"Action":"run","Package":"p","Test":"TestA/one"}`,
		`{"Action":"pause","Package":"p","Test":"TestA/one"}`,
	)
	if h.check(time.Now().Add(2 * time.Minute)) {
		t.Fatal("a parent with queued subtests was flagged")
	}
}

func TestHangWatcher_FlagsARunningSubtestOfAParent(t *testing.T) {
	h := newHangWatcher(time.Minute)
	feed(t, h,
		`{"Action":"run","Package":"p","Test":"TestA"}`,
		`{"Action":"run","Package":"p","Test":"TestA/one"}`,
	)
	if !h.check(time.Now().Add(2 * time.Minute)) {
		t.Fatal("a subtest past the limit was not flagged")
	}
	if h.hung.test != "TestA/one" {
		t.Errorf("hung = %q, want the subtest", h.hung.test)
	}
}
