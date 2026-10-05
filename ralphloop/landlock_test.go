package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireLandLock_FailsFastWhenHeld(t *testing.T) {
	dir := t.TempDir()
	if err := AcquireLandLock(dir); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := AcquireLandLock(dir); !errors.Is(err, ErrLandLocked) {
		t.Fatalf("second acquire = %v, want ErrLandLocked", err)
	}
	if err := ReleaseLandLock(dir); err != nil {
		t.Fatal(err)
	}
	if err := AcquireLandLock(dir); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
}

func TestLandMarker_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if m, err := ReadLandMarker(dir); err != nil || m != nil {
		t.Fatalf("empty read = %v, %v", m, err)
	}
	want := LandMarker{Epic: "e", Ticket: "06", SourceRange: "a..b", PrePickHead: "abc123"}
	if err := WriteLandMarker(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLandMarker(dir)
	if err != nil || got == nil || *got != want {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}

func TestLandPreflight_ThreeWay(t *testing.T) {
	dir := t.TempDir()
	if v, _ := LandPreflight(dir, "e", "06", false); v != LandClear {
		t.Errorf("no marker, no pick = %v, want LandClear", v)
	}
	if v, _ := LandPreflight(dir, "e", "06", true); v != LandUnknownPick {
		t.Errorf("pick without marker = %v, want LandUnknownPick", v)
	}
	if err := WriteLandMarker(dir, LandMarker{Epic: "e", Ticket: "06"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := LandPreflight(dir, "e", "06", true); v != LandConflictPending {
		t.Errorf("own marker = %v, want LandConflictPending", v)
	}
	if v, _ := LandPreflight(dir, "e", "07", true); v != LandOtherTicket {
		t.Errorf("other ticket's marker = %v, want LandOtherTicket", v)
	}
}

func TestClearLand_RemovesMarkerAndLock(t *testing.T) {
	dir := t.TempDir()
	if err := AcquireLandLock(dir); err != nil {
		t.Fatal(err)
	}
	if err := WriteLandMarker(dir, LandMarker{Epic: "e", Ticket: "06"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearLand(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{landLockFile, landMarkerFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s still present (err=%v)", name, err)
		}
	}
	if err := ClearLand(dir); err != nil {
		t.Errorf("clearing twice: %v", err)
	}
}

func TestLandLock_RecordsOwner(t *testing.T) {
	dir := t.TempDir()
	if o, err := ReadLandLock(dir); err != nil || o != nil {
		t.Fatalf("no lock: owner = %v, err = %v", o, err)
	}
	if err := AcquireLandLockFor(dir, "epic", "07"); err != nil {
		t.Fatal(err)
	}
	o, err := ReadLandLock(dir)
	if err != nil || o == nil {
		t.Fatalf("owner = %v, err = %v", o, err)
	}
	if o.PID != os.Getpid() || o.Epic != "epic" || o.Ticket != "07" || o.Time.IsZero() || !o.Alive() {
		t.Errorf("owner = %+v", o)
	}
}

func TestOrphanLandLock_NeedsLockAndNoMarker(t *testing.T) {
	dir := t.TempDir()
	if o, _ := OrphanLandLock(dir); o != nil {
		t.Fatal("orphan reported with no lock")
	}
	if err := AcquireLandLock(dir); err != nil {
		t.Fatal(err)
	}
	if o, _ := OrphanLandLock(dir); o == nil {
		t.Fatal("lock without marker not reported")
	}
	if err := WriteLandMarker(dir, LandMarker{Epic: "e", Ticket: "1"}); err != nil {
		t.Fatal(err)
	}
	if o, _ := OrphanLandLock(dir); o != nil {
		t.Fatal("lock with marker reported as orphan")
	}
}
