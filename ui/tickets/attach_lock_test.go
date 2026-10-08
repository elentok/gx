package tickets

import (
	"encoding/json"
	"os"
	"path/filepath"
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
