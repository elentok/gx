package ralphloop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	landLockFile   = "land.lock"
	landMarkerFile = "land.marker.json"
)

// ErrLandLocked is returned when the land lock is already held. Its reason code
// in the recovery JSON envelope is "land_locked".
var ErrLandLocked = errors.New("land lock is held")

// errLandDeferred means a land must wait: a human's conflict is pending.
var errLandDeferred = errors.New("land deferred")

// LandMarker records a conflict a land left behind, so a human's pending
// resolution can be told apart from stale crash state.
type LandMarker struct {
	Epic        string `json:"epic"`
	Ticket      string `json:"ticket"`
	SourceRange string `json:"source_range"`
	PrePickHead string `json:"pre_pick_head"`
}

// LandVerdict is the outcome of the three-way land preflight.
type LandVerdict int

const (
	LandClear           LandVerdict = iota // nothing pending, safe to land
	LandConflictPending                    // marker for this ticket: --continue / --abort
	LandOtherTicket                        // marker for a different ticket: refuse
	LandUnknownPick                        // cherry-pick in progress, no marker: refuse
)

// AcquireLandLock creates the lock file in dir. It is an existence lock rather
// than a flock: a conflicted land exits while still holding it, and the lock
// must outlive that process until --continue or --abort clears it. It never
// blocks; a held lock returns ErrLandLocked.
func AcquireLandLock(dir string) error {
	return AcquireLandLockFor(dir, "", "")
}

// LandLockOwner is who took the land lock, so a lock a crash left behind can be
// told apart from a live land.
type LandLockOwner struct {
	PID    int       `json:"pid"`
	Time   time.Time `json:"time"`
	Epic   string    `json:"epic,omitempty"`
	Ticket string    `json:"ticket,omitempty"`
}

// Alive reports whether the owning process still exists.
func (o LandLockOwner) Alive() bool {
	if o.PID <= 0 {
		return false
	}
	p, err := os.FindProcess(o.PID)
	if err != nil {
		return false
	}
	// EPERM means the process exists but belongs to someone else.
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Describe is the human-readable owner, for refusals and verify output.
func (o LandLockOwner) Describe() string {
	s := fmt.Sprintf("pid %d since %s", o.PID, o.Time.Format(time.RFC3339))
	if o.Ticket != "" {
		s += fmt.Sprintf(" (%s/%s)", o.Epic, o.Ticket)
	}
	return s
}

// AcquireLandLockFor is AcquireLandLock recording epic/ticket as the owner.
func AcquireLandLockFor(dir, epic, ticket string) error {
	f, err := os.OpenFile(filepath.Join(dir, landLockFile), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if errors.Is(err, os.ErrExist) {
		return ErrLandLocked
	}
	if err != nil {
		return err
	}
	b, err := json.Marshal(LandLockOwner{PID: os.Getpid(), Time: time.Now().UTC(), Epic: epic, Ticket: ticket})
	if err == nil {
		_, err = f.Write(b)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// ReadLandLock returns the lock's owner, or nil when no lock exists. A lock
// whose owner can't be read (a crash mid-write) yields a zero owner, which is
// never Alive.
func ReadLandLock(dir string) (*LandLockOwner, error) {
	b, err := os.ReadFile(filepath.Join(dir, landLockFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var o LandLockOwner
	if json.Unmarshal(b, &o) != nil {
		o = LandLockOwner{}
	}
	return &o, nil
}

// OrphanLandLock returns the lock's owner when a lock exists with no marker:
// either a land in flight or one a crash left behind (see Alive).
func OrphanLandLock(dir string) (*LandLockOwner, error) {
	m, err := ReadLandMarker(dir)
	if err != nil || m != nil {
		return nil, err
	}
	return ReadLandLock(dir)
}

// ReleaseLandLock removes the lock without touching the marker. A missing lock
// is not an error.
func ReleaseLandLock(dir string) error {
	return removeIfExists(filepath.Join(dir, landLockFile))
}

// WriteLandMarker writes the marker beside the lock.
func WriteLandMarker(dir string, m LandMarker) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, landMarkerFile+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, landMarkerFile)); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// ReadLandMarker returns the marker, or nil when none exists.
func ReadLandMarker(dir string) (*LandMarker, error) {
	b, err := os.ReadFile(filepath.Join(dir, landMarkerFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m LandMarker
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ClearLand removes the marker and the lock.
func ClearLand(dir string) error {
	if err := removeIfExists(filepath.Join(dir, landMarkerFile)); err != nil {
		return err
	}
	return ReleaseLandLock(dir)
}

// LandPreflight decides whether epic/ticket may land, given the marker in dir
// and whether a cherry-pick is in progress.
func LandPreflight(dir, epic, ticket string, pickInProgress bool) (LandVerdict, error) {
	m, err := ReadLandMarker(dir)
	if err != nil {
		return LandClear, err
	}
	switch {
	case m != nil && m.Epic == epic && m.Ticket == ticket:
		return LandConflictPending, nil
	case m != nil:
		return LandOtherTicket, nil
	case pickInProgress:
		return LandUnknownPick, nil
	}
	return LandClear, nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
