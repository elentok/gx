package disk

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// cleanNow runs the disk's cleanup work in the caller and ends it through the
// same rule as the automatic clean. It never touches armed/spent and never sets
// the hold at its start.
func (s *Disk) cleanNow(id string) wire.CleanNowResult {
	d := &s.disks
	d.mu.Lock()
	st := d.disks[id]
	switch {
	case st == nil:
		d.mu.Unlock()
		return wire.CleanNowResult{Refused: true, Reason: wire.ReasonUnknownDisk, Message: "unknown disk " + id}
	case st.cleaning:
		d.mu.Unlock()
		return wire.CleanNowResult{Refused: true, Reason: wire.ReasonCleanRunning, Message: "clean already running"}
	}
	probe, projects, before := st.probe, st.projects, st.reading.Free
	st.beginClean(before)
	d.mu.Unlock()
	s.events.Publish(wire.EventDiskWarnChanged, "")

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
	if len(work) == 0 {
		s.sweepTemp()
	}
	end := cleanEnd{failed: s.runCleanWork(work), noWork: len(work) == 0}
	r, held := s.endClean(id, probe, end)
	return wire.CleanNowResult{FreedBytes: max(r.Free, before) - before, FreeBytes: r.Free, Held: held, Failure: end.failed, NoCleanupCommand: end.noWork}
}

func (s *Disk) cleanWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.CleanNowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	wire.WriteJSON(w, s.cleanNow(req.Disk))
}
