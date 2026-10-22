package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireMergeLock_FailsFastWhenHeld(t *testing.T) {
	dir := t.TempDir()
	if err := AcquireMergeLock(dir); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := AcquireMergeLock(dir); !errors.Is(err, ErrMergeLocked) {
		t.Fatalf("second acquire = %v, want ErrMergeLocked", err)
	}
	if err := ReleaseMergeLock(dir); err != nil {
		t.Fatal(err)
	}
	if err := AcquireMergeLock(dir); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
}

func TestMergeMarker_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if m, err := ReadMergeMarker(dir); err != nil || m != nil {
		t.Fatalf("empty read = %v, %v", m, err)
	}
	want := MergeMarker{Epic: "e", Ticket: "06", SourceRange: "a..b", PrePickHead: "abc123"}
	if err := WriteMergeMarker(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMergeMarker(dir)
	if err != nil || got == nil || *got != want {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}

func TestMergePreflight_ThreeWay(t *testing.T) {
	dir := t.TempDir()
	if v, _ := MergePreflight(dir, "e", "06", false); v != MergeClear {
		t.Errorf("no marker, no pick = %v, want MergeClear", v)
	}
	if v, _ := MergePreflight(dir, "e", "06", true); v != MergeUnknownPick {
		t.Errorf("pick without marker = %v, want MergeUnknownPick", v)
	}
	if err := WriteMergeMarker(dir, MergeMarker{Epic: "e", Ticket: "06"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := MergePreflight(dir, "e", "06", true); v != MergeConflictPending {
		t.Errorf("own marker = %v, want MergeConflictPending", v)
	}
	if v, _ := MergePreflight(dir, "e", "07", true); v != MergeOtherTicket {
		t.Errorf("other ticket's marker = %v, want MergeOtherTicket", v)
	}
}

func TestClearMerge_RemovesMarkerAndLock(t *testing.T) {
	dir := t.TempDir()
	if err := AcquireMergeLock(dir); err != nil {
		t.Fatal(err)
	}
	if err := WriteMergeMarker(dir, MergeMarker{Epic: "e", Ticket: "06"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearMerge(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{mergeLockFile, mergeMarkerFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s still present (err=%v)", name, err)
		}
	}
	if err := ClearMerge(dir); err != nil {
		t.Errorf("clearing twice: %v", err)
	}
}

func TestMergeLock_RecordsOwner(t *testing.T) {
	dir := t.TempDir()
	if o, err := ReadMergeLock(dir); err != nil || o != nil {
		t.Fatalf("no lock: owner = %v, err = %v", o, err)
	}
	if err := AcquireMergeLockFor(dir, "epic", "07"); err != nil {
		t.Fatal(err)
	}
	o, err := ReadMergeLock(dir)
	if err != nil || o == nil {
		t.Fatalf("owner = %v, err = %v", o, err)
	}
	if o.PID != os.Getpid() || o.Epic != "epic" || o.Ticket != "07" || o.Time.IsZero() || !o.Alive() {
		t.Errorf("owner = %+v", o)
	}
}

func TestOrphanMergeLock_NeedsLockAndNoMarker(t *testing.T) {
	dir := t.TempDir()
	if o, _ := OrphanMergeLock(dir); o != nil {
		t.Fatal("orphan reported with no lock")
	}
	if err := AcquireMergeLock(dir); err != nil {
		t.Fatal(err)
	}
	if o, _ := OrphanMergeLock(dir); o == nil {
		t.Fatal("lock without marker not reported")
	}
	if err := WriteMergeMarker(dir, MergeMarker{Epic: "e", Ticket: "1"}); err != nil {
		t.Fatal(err)
	}
	if o, _ := OrphanMergeLock(dir); o != nil {
		t.Fatal("lock with marker reported as orphan")
	}
}
