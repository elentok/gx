package tickets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFakeProcessStartTime swaps processStartTime for a lookup table keyed
// by pid, restoring the real implementation on cleanup.
func withFakeProcessStartTime(t *testing.T, table map[int]string) {
	t.Helper()
	previous := processStartTime
	processStartTime = func(pid int) (string, bool) {
		s, alive := table[pid]
		return s, alive
	}
	t.Cleanup(func() { processStartTime = previous })
}

func readAttachLockFile(t *testing.T, scratchDir string) attachLockInfo {
	t.Helper()
	raw, err := os.ReadFile(attachLockPath(scratchDir))
	if err != nil {
		t.Fatalf("reading attach lock: %v", err)
	}
	var info attachLockInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("unmarshalling attach lock: %v", err)
	}
	return info
}

func TestAcquireAttachLockWhenUnattachedSucceeds(t *testing.T) {
	// not parallel-safe: reassigns the package-level processStartTime singleton
	dir := t.TempDir()
	withFakeProcessStartTime(t, map[int]string{os.Getpid(): "self-start-1"})

	foreignPID, ok, err := acquireAttachLock(dir)
	if err != nil || !ok {
		t.Fatalf("acquireAttachLock() = (%d, %v, %v), want (_, true, nil)", foreignPID, ok, err)
	}

	info := readAttachLockFile(t, dir)
	if info.PID != os.Getpid() || info.StartTime != "self-start-1" {
		t.Fatalf("lock file = %#v, want pid=%d start_time=self-start-1", info, os.Getpid())
	}
}

func TestAcquireAttachLockForeignLiveBlocks(t *testing.T) {
	// not parallel-safe: reassigns the package-level processStartTime singleton
	dir := t.TempDir()
	foreignPID := 424242
	withFakeProcessStartTime(t, map[int]string{
		os.Getpid(): "self-start-1",
		foreignPID:  "foreign-start-1",
	})

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAttachLockFile(t, dir, attachLockInfo{PID: foreignPID, StartTime: "foreign-start-1"})

	gotPID, ok, err := acquireAttachLock(dir)
	if err != nil || ok {
		t.Fatalf("acquireAttachLock() = (%d, %v, %v), want (%d, false, nil)", gotPID, ok, err, foreignPID)
	}
	if gotPID != foreignPID {
		t.Fatalf("acquireAttachLock() foreignPID = %d, want %d", gotPID, foreignPID)
	}

	info := readAttachLockFile(t, dir)
	if info.PID != foreignPID {
		t.Fatalf("lock file pid = %d, want untouched foreign pid %d", info.PID, foreignPID)
	}
}

func TestAcquireAttachLockReclaimsDeadPid(t *testing.T) {
	// not parallel-safe: reassigns the package-level processStartTime singleton
	dir := t.TempDir()
	deadPID := 424243
	withFakeProcessStartTime(t, map[int]string{
		os.Getpid(): "self-start-1",
		// deadPID absent from table => processStartTime reports not alive.
	})

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAttachLockFile(t, dir, attachLockInfo{PID: deadPID, StartTime: "long-gone"})

	gotPID, ok, err := acquireAttachLock(dir)
	if err != nil || !ok {
		t.Fatalf("acquireAttachLock() = (%d, %v, %v), want (_, true, nil)", gotPID, ok, err)
	}

	info := readAttachLockFile(t, dir)
	if info.PID != os.Getpid() {
		t.Fatalf("lock file pid = %d, want reclaimed by this process (%d)", info.PID, os.Getpid())
	}
}

func TestAcquireAttachLockReclaimsMismatchedStartTime(t *testing.T) {
	// not parallel-safe: reassigns the package-level processStartTime singleton
	dir := t.TempDir()
	reusedPID := 424244
	withFakeProcessStartTime(t, map[int]string{
		os.Getpid(): "self-start-1",
		reusedPID:   "current-start-time",
	})

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAttachLockFile(t, dir, attachLockInfo{PID: reusedPID, StartTime: "old-start-time"})

	gotPID, ok, err := acquireAttachLock(dir)
	if err != nil || !ok {
		t.Fatalf("acquireAttachLock() = (%d, %v, %v), want (_, true, nil)", gotPID, ok, err)
	}

	info := readAttachLockFile(t, dir)
	if info.PID != os.Getpid() {
		t.Fatalf("lock file pid = %d, want reclaimed by this process (%d)", info.PID, os.Getpid())
	}
}

func writeAttachLockFile(t *testing.T, dir string, info attachLockInfo) {
	t.Helper()
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, attachLockFileName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTryStartAcquiresAttachLockOnceAcrossEpics(t *testing.T) {
	// not parallel-safe: reassigns the package-level processStartTime singleton
	dir := t.TempDir()
	withFakeProcessStartTime(t, map[int]string{os.Getpid(): "self-start-1"})

	r := newLoopRegistry(2)
	if _, ok := r.tryStart("epic-a", 0, 1, dir); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	before := readAttachLockFile(t, dir)

	if _, ok := r.tryStart("epic-b", 0, 1, dir); !ok {
		t.Fatal("tryStart(epic-b): want success")
	}
	t.Cleanup(func() { r.finish("epic-b", nil) })
	after := readAttachLockFile(t, dir)
	if before != after {
		t.Fatalf("second tryStart rewrote the lock: before=%#v after=%#v", before, after)
	}
	if r.attachCount != 2 {
		t.Fatalf("attachCount = %d, want 2", r.attachCount)
	}
}

func TestTryStartFailsAndLeavesRegistryUntouchedWhenForeignAttached(t *testing.T) {
	// not parallel-safe: reassigns the package-level processStartTime singleton
	dir := t.TempDir()
	foreignPID := 424245
	withFakeProcessStartTime(t, map[int]string{
		os.Getpid(): "self-start-1",
		foreignPID:  "foreign-start-1",
	})
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAttachLockFile(t, dir, attachLockInfo{PID: foreignPID, StartTime: "foreign-start-1"})

	r := newLoopRegistry(2)
	if _, ok := r.tryStart("epic-a", 0, 1, dir); ok {
		t.Fatal("tryStart(epic-a): want rejection, foreign process holds attach lock")
	}
	if len(r.runs) != 0 || len(r.snapshots) != 0 {
		t.Fatalf("registry mutated on rejected start: runs=%#v snapshots=%#v", r.runs, r.snapshots)
	}
	err := r.takeAttachError()
	if err == nil {
		t.Fatal("takeAttachError(): want non-nil error naming the foreign pid")
	}
	if want := "424245"; !strings.Contains(err.Error(), want) {
		t.Fatalf("attach error = %q, want it to name pid %s", err.Error(), want)
	}
}

func TestSelfAttachedReflectsThisProcessOnly(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/processStartTime singletons
	dir := t.TempDir()
	withFakeProcessStartTime(t, map[int]string{os.Getpid(): "self-start-1"})

	r := newLoopRegistry(2)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() { ralphLoopRegistry = previous })

	if SelfAttached() {
		t.Fatal("SelfAttached() = true before tryStart, want false")
	}
	if _, ok := r.tryStart("epic-a", 0, 1, dir); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	if !SelfAttached() {
		t.Fatal("SelfAttached() = false while this process holds the lock, want true")
	}
	r.finish("epic-a", nil)
	if SelfAttached() {
		t.Fatal("SelfAttached() = true after finish released the lock, want false")
	}
}

func TestForeignAttachPIDReportsLiveForeignHolderOnly(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/processStartTime singletons
	dir := t.TempDir()
	foreignPID := 424246
	withFakeProcessStartTime(t, map[int]string{
		os.Getpid(): "self-start-1",
		foreignPID:  "foreign-start-1",
	})

	if got := ForeignAttachPID(dir); got != 0 {
		t.Fatalf("ForeignAttachPID() = %d before any lock exists, want 0", got)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAttachLockFile(t, dir, attachLockInfo{PID: foreignPID, StartTime: "foreign-start-1"})
	if got := ForeignAttachPID(dir); got != foreignPID {
		t.Fatalf("ForeignAttachPID() = %d, want live foreign pid %d", got, foreignPID)
	}

	r := newLoopRegistry(2)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() { ralphLoopRegistry = previous })
	selfDir := t.TempDir()
	if _, ok := r.tryStart("epic-a", 0, 1, selfDir); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	if got := ForeignAttachPID(dir); got != 0 {
		t.Fatalf("ForeignAttachPID() = %d while this process holds its own lock, want 0", got)
	}
}

func TestFinishReleasesAttachLockOnlyWhenLastRunEnds(t *testing.T) {
	// not parallel-safe: reassigns the package-level processStartTime singleton
	dir := t.TempDir()
	withFakeProcessStartTime(t, map[int]string{os.Getpid(): "self-start-1"})

	r := newLoopRegistry(2)
	r.tryStart("epic-a", 0, 1, dir)
	r.tryStart("epic-b", 0, 1, dir)

	r.finish("epic-a", nil)
	if _, err := os.Stat(attachLockPath(dir)); err != nil {
		t.Fatalf("attach lock removed while epic-b still running: %v", err)
	}
	if r.attachCount != 1 {
		t.Fatalf("attachCount after first finish = %d, want 1", r.attachCount)
	}

	r.finish("epic-b", nil)
	if _, err := os.Stat(attachLockPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("attach lock still present after last run finished: err=%v", err)
	}
	if r.attachCount != 0 {
		t.Fatalf("attachCount after last finish = %d, want 0", r.attachCount)
	}
}
