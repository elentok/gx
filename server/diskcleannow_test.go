package server_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

const cleanDisk = "d1"

func (r *cleanRig) cleanNow(t *testing.T) server.CleanNowResult {
	t.Helper()
	res, err := r.h.Client.CleanDisk(context.Background(), cleanDisk)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// cleanNowAsync starts a "clean now" that blocks on the runner.
func (r *cleanRig) cleanNowAsync() <-chan server.CleanNowResult {
	done := make(chan server.CleanNowResult, 1)
	go func() {
		res, _ := r.h.Client.CleanDisk(context.Background(), cleanDisk)
		done <- res
	}()
	return done
}

// known waits for the watcher to have read the disk once.
func (r *cleanRig) known(t *testing.T) {
	t.Helper()
	waitFor(t, func() bool {
		rep, err := r.h.Client.DiskStatus(context.Background())
		if err != nil {
			return false
		}
		for _, d := range rep.Disks {
			if d.ID == cleanDisk {
				return true
			}
		}
		return false
	})
}

func (r *cleanRig) afterEachRun(f func()) {
	r.clean.mu.Lock()
	r.clean.after = f
	r.clean.mu.Unlock()
}

// Story 37.
func TestDiskCleanNow_RunsTheSameWorkAndStaysArmed(t *testing.T) {
	r := startCleanRig(t, map[string]string{"plain": "", "custom": overrideCmd}, 0, 10*gb)
	r.known(t)
	res := r.cleanNow(t)
	if res.Refused || res.Held || res.NoCleanupCommand {
		t.Fatalf("result = %+v", res)
	}
	want := []string{"go clean -cache @ " + r.repos["plain"], "make tidy @ " + r.repos["custom"]}
	got := r.clean.recorded()
	if len(got) != 2 || !(got[0] == want[0] && got[1] == want[1] || got[0] == want[1] && got[1] == want[0]) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	r.setFree(3 * gb)
	r.waitCalls(t, 4)
}

// Story 38, held disk.
func TestDiskCleanNow_HoldEndsInTheResponse(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
	r.setFree(3 * gb)
	r.waitClean(t, 1)
	r.disksWhen(t, "held", func(d []server.DiskStatus) bool { return len(d) == 1 && d[0].Held && !d[0].Cleaning })
	r.afterEachRun(func() { r.setFree(50 * gb) })
	res := r.cleanNow(t)
	if res.Refused || res.Held || res.FreeBytes != 50*gb || res.FreedBytes != 47*gb {
		t.Fatalf("result = %+v", res)
	}
	snap, err := r.h.Client.Snapshot(context.Background())
	if err != nil || len(snap.Disks) != 0 {
		t.Errorf("snapshot disks = %+v, %v; want none", snap.Disks, err)
	}
	r.waitClean(t, 2)
	letWatcherRead()
	if n := r.countBodies("disk clean"); n != 2 {
		t.Errorf("clean notices = %d, want 2", n)
	}
	if r.countBodies("dispatch resumed") != 0 {
		t.Error("a resume notice was sent")
	}
}

// Story 38, warned disk.
func TestDiskCleanNow_UnheldDiskKeepsDispatching(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 20*gb, 10*gb, servertest.WithHerdr(servertest.UntilPtyFixture))
	r.setFree(15 * gb)
	r.known(t)
	r.clean.blockUntilCleanup(t)
	done := r.cleanNowAsync()
	r.waitCalls(t, 1)
	r.queue(t, "proj:epic-a/01")
	waitFor(t, func() bool { return len(*r.start) >= 1 })
	r.clean.release()
	if res := <-done; res.Refused || res.Held {
		t.Fatalf("result = %+v", res)
	}
	body := r.waitClean(t, 1)
	if !strings.Contains(body, "dispatch is not held") {
		t.Errorf("notice = %s, want not held", body)
	}
	if r.countBodies("dispatch resumed") != 0 {
		t.Error("a resume notice was sent")
	}
}

// Story 39.
func TestDiskCleanNow_NoCleanupWorkOnlyReadsAgain(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": noCmd}, 0, 10*gb)
	r.setFree(3 * gb)
	r.waitClean(t, 1)
	r.setFree(50 * gb)
	res := r.cleanNow(t)
	if res.Refused || res.Held || res.FreeBytes != 50*gb || !res.NoCleanupCommand {
		t.Fatalf("result = %+v", res)
	}
	r.wantCalls(t, 0)
	if body := r.waitClean(t, 2); !strings.Contains(body, "no cleanup command set") || strings.Contains(body, "freed") {
		t.Errorf("notice = %s, want no cleanup command set", body)
	}
}

// Story 41.
func TestDiskCleanNow_Refusals(t *testing.T) {
	t.Run("automatic clean running", func(t *testing.T) {
		r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
		r.clean.blockUntilCleanup(t)
		r.setFree(3 * gb)
		r.waitCalls(t, 1)
		res := r.cleanNow(t)
		if !res.Refused || res.Reason != server.ReasonCleanRunning || res.Message != "clean already running" {
			t.Errorf("result = %+v", res)
		}
		r.clean.release()
	})
	t.Run("clean now running", func(t *testing.T) {
		r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
		r.known(t)
		r.clean.blockUntilCleanup(t)
		done := r.cleanNowAsync()
		r.waitCalls(t, 1)
		res := r.cleanNow(t)
		if !res.Refused || res.Message != "clean already running" {
			t.Errorf("result = %+v", res)
		}
		r.clean.release()
		if first := <-done; first.Refused {
			t.Errorf("first = %+v", first)
		}
	})
	t.Run("unknown disk", func(t *testing.T) {
		r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
		res, err := r.h.Client.CleanDisk(context.Background(), "nope")
		if err != nil || !res.Refused || res.Reason != server.ReasonUnknownDisk {
			t.Errorf("result = %+v, %v", res, err)
		}
	})
}

// Story 41: a "clean now" never changes armed or spent.
func TestDiskCleanNow_ArmedStaysArmed(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
	r.known(t)
	r.clean.blockUntilCleanup(t)
	done := r.cleanNowAsync()
	r.waitCalls(t, 1)
	r.setFree(3 * gb)
	r.disksWhen(t, "held", func(d []server.DiskStatus) bool { return len(d) == 1 && d[0].Held })
	r.wantCalls(t, 1)
	r.clean.release()
	if res := <-done; !res.Held {
		t.Fatalf("result = %+v, want held", res)
	}
	r.waitCalls(t, 2)
}

func TestDiskCleanNow_FailureIsAnAnswer(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
	r.known(t)
	r.clean.err = errors.New("boom")
	res := r.cleanNow(t)
	if res.Refused || !strings.Contains(res.Failure, "go clean -cache") {
		t.Fatalf("result = %+v", res)
	}
	if body := r.waitClean(t, 1); !strings.Contains(body, "failed") {
		t.Errorf("notice = %s, want failed", body)
	}
}

func TestWebDiskClean_PostReachesTheHandler(t *testing.T) {
	h, base := startWeb(t)
	cookie := login(t, h.StateDir, base)
	code, body := webPostJSON(t, base+"/v1/disk/clean", `{"disk":"nope"}`, cookie, "", jsonPost(base))
	if code != http.StatusOK || reasonOf(t, body) != server.ReasonUnknownDisk {
		t.Errorf("web disk clean = %d %s", code, body)
	}
}
