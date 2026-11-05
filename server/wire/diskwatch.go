package wire

import (
	"time"
)

const (
	// NoticeDiskWarn is the notify kind for a disk that dropped under the warn threshold.
	NoticeDiskWarn = "disk-warn"
	// EventDiskWarnChanged is emitted when a disk's warn mark is set or cleared.
	EventDiskWarnChanged = "disk-warn-changed"
	// NoticeDiskResume is the notify kind for a held disk that climbed back to the clean threshold.
	NoticeDiskResume = "disk-resume"
	// VerdictDiskLow is the explain verdict for a queued ticket whose project watches a held disk.
	VerdictDiskLow = "disk low"
	// ReasonDiskLow refuses a relaunch while a disk of the ticket's project is held.
	ReasonDiskLow = "disk-low"
)

// DiskReading is one free-space reading of a disk (a volume, not a path).
type DiskReading struct {
	ID   string // device id, shared by every path on the volume
	Name string // mount point or volume name
	Free uint64 // bytes available to a non-root user
	// Total is the size of the volume in bytes.
	Total uint64
}

// DiskStatus is one disk, in the snapshot (warned or held disks only) and in
// the disk status route (every known disk).
type DiskStatus struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	FreeBytes  uint64    `json:"free_bytes"`
	TotalBytes uint64    `json:"total_bytes"`
	Projects   []string  `json:"projects"`
	Warned     bool      `json:"warned"`
	Held       bool      `json:"held"`
	ReadAt     time.Time `json:"read_at"`
	Cleaning   bool      `json:"cleaning"`
	LastClean  string    `json:"last_clean"` // bytes freed, the failure, or that no command is set
}

// DiskReport is the disk status route's result: the effective thresholds as
// size strings ("off" when a threshold is off) and every known disk.
type DiskReport struct {
	WarnBelow  string       `json:"warn_below"`
	CleanBelow string       `json:"clean_below"`
	Disks      []DiskStatus `json:"disks"`
}

const (
	// ReasonUnknownDisk refuses a "clean now" for a disk the watcher does not know.
	ReasonUnknownDisk = "unknown-disk"
	// ReasonCleanRunning refuses a "clean now" while any clean runs on the disk.
	ReasonCleanRunning = "clean-running"
)

// CleanNowRequest names the disk to clean.
type CleanNowRequest struct {
	Disk string `json:"disk"`
}

// CleanNowResult is the answer to a "clean now". A failed command is an answer
// (Failure set), not a refusal.
type CleanNowResult struct {
	FreedBytes uint64 `json:"freed_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	Held       bool   `json:"held"`
	Failure    string `json:"failure,omitempty"`
	// NoCleanupCommand says no project on the disk had cleanup work to run.
	NoCleanupCommand bool   `json:"no_cleanup_command,omitempty"`
	Refused          bool   `json:"refused,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Message          string `json:"message,omitempty"`
}
