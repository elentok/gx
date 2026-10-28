package server

import (
	"encoding/json"
	"net/http"
)

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
	Refused    bool   `json:"refused,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
}

// cleanNow runs the disk's cleanup work in the caller and ends it through the
// same rule as the automatic clean. It never touches armed/spent and never sets
// the hold at its start.
func (s *Server) cleanNow(id string) CleanNowResult {
	d := &s.disks
	d.mu.Lock()
	st := d.disks[id]
	switch {
	case st == nil:
		d.mu.Unlock()
		return CleanNowResult{Refused: true, Reason: ReasonUnknownDisk, Message: "unknown disk " + id}
	case st.cleaning:
		d.mu.Unlock()
		return CleanNowResult{Refused: true, Reason: ReasonCleanRunning, Message: "clean already running"}
	}
	probe, projects, before := st.probe, st.projects, st.reading.Free
	st.beginClean(before)
	d.mu.Unlock()
	s.events.publish(EventDiskWarnChanged, "")

	// The fresh read, not the last poll, is what "freed" is measured from.
	if r, err := s.freeSpace(probe); err != nil {
		s.log.Warn("read free space", "path", probe, "err", err)
	} else {
		before = r.Free
		d.mu.Lock()
		st.cleanFree = before
		d.mu.Unlock()
	}

	work := s.cleanWork(projects)
	end := cleanEnd{failed: s.runCleanWork(work), noWork: len(work) == 0}
	r, held := s.endClean(id, probe, end)
	return CleanNowResult{FreedBytes: max(r.Free, before) - before, FreeBytes: r.Free, Held: held, Failure: end.failed}
}

func (s *Server) diskCleanWrite(w http.ResponseWriter, r *http.Request) {
	var req CleanNowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, s.cleanNow(req.Disk))
}
