package wire

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
