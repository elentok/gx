package ralphloop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/elentok/gx/tickets"
)

const (
	// On-disk names keep the old word so locks left by an older binary are
	// still found and recovered.
	mergeLockFile   = "land.lock"
	mergeMarkerFile = "land.marker.json"
)

// MergeLockDirIn is where an epic's merge lock and marker live: under
// stateDir, keyed by the epic's project and name, so every process merging
// onto the epic's feature branch (CLI or server) contends for the same lock
// whichever path it reached the epic by. An empty stateDir is an error.
func MergeLockDirIn(stateDir, epicPath string) (string, error) {
	if stateDir == "" {
		return "", errors.New("resolving merge lock dir: no state dir configured")
	}
	epicPath = filepath.Clean(epicPath)
	return filepath.Join(stateDir, "land-locks", tickets.ProjectName(filepath.Dir(epicPath)), filepath.Base(epicPath)), nil
}

// ErrMergeLocked is returned when the merge lock is already held. Its reason code
// in the recovery JSON envelope is "merge_locked".
var ErrMergeLocked = errors.New("merge lock is held")

// errMergeDeferred means a merge must wait: a human's conflict is pending.
var errMergeDeferred = errors.New("merge deferred")

// MergeMarker records a conflict a merge left behind, so a human's pending
// resolution can be told apart from stale crash state.
type MergeMarker struct {
	Epic        string `json:"epic"`
	Ticket      string `json:"ticket"`
	SourceRange string `json:"source_range"`
	PrePickHead string `json:"pre_pick_head"`
}

// MergeVerdict is the outcome of the three-way merge preflight.
type MergeVerdict int

const (
	MergeClear           MergeVerdict = iota // nothing pending, safe to merge
	MergeConflictPending                     // marker for this ticket: --continue / --abort
	MergeOtherTicket                         // marker for a different ticket: refuse
	MergeUnknownPick                         // cherry-pick in progress, no marker: refuse
)

// AcquireMergeLock creates the lock file in dir. It is an existence lock rather
// than a flock: a conflicted merge exits while still holding it, and the lock
// must outlive that process until --continue or --abort clears it. It never
// blocks; a held lock returns ErrMergeLocked.
func AcquireMergeLock(dir string) error {
	return AcquireMergeLockFor(dir, "", "")
}

// MergeLockOwner is who took the merge lock, so a lock a crash left behind can be
// told apart from a live merge.
type MergeLockOwner struct {
	PID    int       `json:"pid"`
	Time   time.Time `json:"time"`
	Epic   string    `json:"epic,omitempty"`
	Ticket string    `json:"ticket,omitempty"`
	// PrePickHead is the feature branch's HEAD when the lock was taken. A
	// dead owner's lock plus a branch that moved past it is how startup
	// finds a merging git finished but gx never recorded.
	PrePickHead string `json:"pre_pick_head,omitempty"`
}

// Alive reports whether the owning process still exists.
func (o MergeLockOwner) Alive() bool {
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
func (o MergeLockOwner) Describe() string {
	s := fmt.Sprintf("pid %d since %s", o.PID, o.Time.Format(time.RFC3339))
	if o.Ticket != "" {
		s += fmt.Sprintf(" (%s/%s)", o.Epic, o.Ticket)
	}
	return s
}

// AcquireMergeLockFor is AcquireMergeLock recording epic/ticket as the owner.
func AcquireMergeLockFor(dir, epic, ticket string) error {
	return AcquireMergeLockAt(dir, epic, ticket, "")
}

// AcquireMergeLockAt is AcquireMergeLockFor also recording the feature
// branch's pre-pick HEAD.
func AcquireMergeLockAt(dir, epic, ticket, prePickHead string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, mergeLockFile), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if errors.Is(err, os.ErrExist) {
		return ErrMergeLocked
	}
	if err != nil {
		return err
	}
	b, err := json.Marshal(MergeLockOwner{PID: os.Getpid(), Time: time.Now().UTC(), Epic: epic, Ticket: ticket, PrePickHead: prePickHead})
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

// ReadMergeLock returns the lock's owner, or nil when no lock exists. A lock
// whose owner can't be read (a crash mid-write) yields a zero owner, which is
// never Alive.
func ReadMergeLock(dir string) (*MergeLockOwner, error) {
	b, err := os.ReadFile(filepath.Join(dir, mergeLockFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var o MergeLockOwner
	if json.Unmarshal(b, &o) != nil {
		o = MergeLockOwner{}
	}
	return &o, nil
}

// OrphanMergeLock returns the lock's owner when a lock exists with no marker:
// either a merge in flight or one a crash left behind (see Alive).
func OrphanMergeLock(dir string) (*MergeLockOwner, error) {
	m, err := ReadMergeMarker(dir)
	if err != nil || m != nil {
		return nil, err
	}
	return ReadMergeLock(dir)
}

// ReleaseMergeLock removes the lock without touching the marker. A missing lock
// is not an error.
func ReleaseMergeLock(dir string) error {
	return removeIfExists(filepath.Join(dir, mergeLockFile))
}

// WriteMergeMarker writes the marker beside the lock.
func WriteMergeMarker(dir string, m MergeMarker) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, mergeMarkerFile+".tmp-*")
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
	if err := os.Rename(tmpPath, filepath.Join(dir, mergeMarkerFile)); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// ReadMergeMarker returns the marker, or nil when none exists.
func ReadMergeMarker(dir string) (*MergeMarker, error) {
	b, err := os.ReadFile(filepath.Join(dir, mergeMarkerFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m MergeMarker
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ClearMerge removes the marker and the lock.
func ClearMerge(dir string) error {
	if err := removeIfExists(filepath.Join(dir, mergeMarkerFile)); err != nil {
		return err
	}
	return ReleaseMergeLock(dir)
}

// MergePreflight decides whether epic/ticket may merge, given the marker in dir
// and whether a cherry-pick is in progress.
func MergePreflight(dir, epic, ticket string, pickInProgress bool) (MergeVerdict, error) {
	m, err := ReadMergeMarker(dir)
	if err != nil {
		return MergeClear, err
	}
	switch {
	case m != nil && m.Epic == epic && m.Ticket == ticket:
		return MergeConflictPending, nil
	case m != nil:
		return MergeOtherTicket, nil
	case pickInProgress:
		return MergeUnknownPick, nil
	}
	return MergeClear, nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
