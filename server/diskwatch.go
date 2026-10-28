package server

import (
	"context"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
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

	defaultDiskPoll = time.Minute
	// diskWarnMargin is how far above the warn threshold a disk must climb
	// before its mark clears, so a reading near the threshold does not flap.
	diskWarnMargin = 2 * humanize.GByte
)

// DiskReading is one free-space reading of a disk (a volume, not a path).
type DiskReading struct {
	ID   string // device id, shared by every path on the volume
	Name string // mount point or volume name
	Free uint64 // bytes available to a non-root user
}

// DiskStatus is one disk, in the snapshot (warned or held disks only) and in
// the disk status route (every known disk).
type DiskStatus struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	FreeBytes uint64    `json:"free_bytes"`
	Projects  []string  `json:"projects"`
	Warned    bool      `json:"warned"`
	Held      bool      `json:"held"`
	ReadAt    time.Time `json:"read_at"`
	Cleaning  bool      `json:"cleaning"`
	LastClean string    `json:"last_clean"` // bytes freed, the failure, or that no command is set
}

type diskState struct {
	reading   DiskReading
	at        time.Time
	projects  []string
	warned    bool
	held      bool // dispatch is held: free space dropped under the clean threshold
	spent     bool // the automatic clean ran; it re-arms once free space climbs well above the line
	cleaning  bool // a clean is running; it alone clears the hold
	cleanFree uint64
	lastClean string
	probe     string // a path on the disk, read again when a clean ends
}

func (st *diskState) status(id string) DiskStatus {
	return DiskStatus{ID: id, Name: st.reading.Name, FreeBytes: st.reading.Free,
		Projects: slices.Clone(st.projects), Warned: st.warned, Held: st.held, ReadAt: st.at,
		Cleaning: st.cleaning, LastClean: st.lastClean}
}

// diskWatch is the per-disk state. Every reading goes through observe, under mu.
type diskWatch struct {
	mu    sync.Mutex
	disks map[string]*diskState

	goCacheOnce sync.Once
	goCachePath string
}

// diskChange is what one reading did to a disk's state, for the effects that
// run after the lock is released.
type diskChange struct {
	changed, warnSet, holdSet, resumed, startClean bool
	held                                           bool
	projects                                       []string
	lastClean                                      string
}

// beginClean marks a clean as running; freeBefore is what "freed" is measured from.
func (st *diskState) beginClean(freeBefore uint64) {
	st.cleaning, st.cleanFree = true, freeBefore
}

// updateWarn sets or clears the warn mark and reports whether it was set now.
func (st *diskState) updateWarn(free, warn uint64) (set, changed bool) {
	switch {
	case !st.warned && warn > 0 && free < warn:
		st.warned = true
		return true, true
	case st.warned && (warn == 0 || free > warn+diskWarnMargin):
		st.warned = false
		return false, true
	}
	return false, false
}

// readTransition applies an ordinary reading: the hold, the re-arm and the
// start of the automatic clean.
func (st *diskState) readTransition(free, warn, clean uint64) diskChange {
	var c diskChange
	c.warnSet, c.changed = st.updateWarn(free, warn)
	if st.spent && free > max(warn, clean+diskWarnMargin) {
		st.spent = false
	}
	low := clean > 0 && free < clean
	switch {
	case !st.held && low:
		st.held, c.changed, c.holdSet = true, true, true
	case st.held && !low && !st.cleaning:
		st.held, c.changed, c.resumed = false, true, true
	}
	if low && !st.spent && !st.cleaning {
		st.spent = true
		st.beginClean(free)
		c.changed, c.startClean = true, true
	}
	return c
}

// endTransition applies the reading that ends a clean. One rule ends every
// clean: the fresh reading decides the hold. It never re-arms; a later reading does.
func (st *diskState) endTransition(free, warn, clean uint64, end cleanEnd) diskChange {
	var c diskChange
	c.warnSet, _ = st.updateWarn(free, warn)
	c.changed = true
	st.cleaning = false
	switch {
	case end.failed != "":
		st.lastClean = "failed: " + end.failed
	case end.noWork:
		st.lastClean = noCleanCommand
	default:
		st.lastClean = "freed " + humanize.Bytes(max(free, st.cleanFree)-st.cleanFree)
	}
	low := clean > 0 && free < clean
	if st.held != low {
		st.held, c.resumed = low, !low
	}
	return c
}

// transition records the reading under the lock, applies fn and returns the
// change with a snapshot of what the effects need. projects is who watches the
// disk now; nil keeps the last list. probe is a path on the disk, read again
// when a clean ends.
func (s *Server) transition(r DiskReading, probe string, projects []string, now time.Time,
	fn func(st *diskState, warn, clean uint64) diskChange) diskChange {
	ds := s.settings.Load().DiskSpace
	d := &s.disks
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.disks[r.ID]
	if st == nil {
		if d.disks == nil {
			d.disks = map[string]*diskState{}
		}
		st = &diskState{}
		d.disks[r.ID] = st
	}
	st.reading, st.at = r, now
	if probe != "" {
		st.probe = probe
	}
	if projects != nil {
		st.projects = projects
	}
	c := fn(st, uint64(ds.WarnBelow), uint64(ds.CleanBelow))
	c.held, c.projects, c.lastClean = st.held, slices.Clone(st.projects), st.lastClean
	return c
}

// observe applies one ordinary reading to the state machine and reports
// whether the disk is held after it.
func (s *Server) observe(r DiskReading, probe string, projects []string, now time.Time) (held bool) {
	c := s.transition(r, probe, projects, now, func(st *diskState, warn, clean uint64) diskChange {
		return st.readTransition(r.Free, warn, clean)
	})
	if !c.changed {
		return c.held
	}
	s.publishChange(r, c)
	if c.resumed {
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeDiskResume, Emoji: "💾", Title: "dispatch resumed on " + r.Name,
			Detail: humanize.Bytes(r.Free) + " free; queued tickets start again"})
		s.kickRunner()
	}
	if c.startClean {
		s.startClean(r.ID, probe, c.projects)
	}
	return c.held
}

// observeCleanEnd applies the reading that ends a clean and reports whether
// the disk is held after it.
func (s *Server) observeCleanEnd(r DiskReading, probe string, end cleanEnd) (held bool) {
	c := s.transition(r, probe, nil, time.Now(), func(st *diskState, warn, clean uint64) diskChange {
		return st.endTransition(r.Free, warn, clean, end)
	})
	s.publishChange(r, c)
	if c.resumed {
		s.kickRunner()
	}
	s.sendCleanNotice(r.Name, c.lastClean, r.Free, c.held)
	return c.held
}

// publishChange announces a changed disk: the event, the warn and hold logs and
// the warn notice.
func (s *Server) publishChange(r DiskReading, c diskChange) {
	ds := s.settings.Load().DiskSpace
	s.events.publish(EventDiskWarnChanged, "")
	if c.warnSet {
		s.log.Warn("disk low on space", "disk", r.Name, "free", r.Free, "warn_below", uint64(ds.WarnBelow))
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeDiskWarn, Emoji: "💾", Title: "low disk space on " + r.Name,
			Detail: humanize.Bytes(r.Free) + " free, under the warn threshold of " + humanize.Bytes(uint64(ds.WarnBelow))})
	}
	if c.holdSet {
		s.log.Warn("disk low, holding dispatch", "disk", r.Name, "free", r.Free, "clean_below", uint64(ds.CleanBelow))
	}
	if c.resumed {
		s.log.Info("disk recovered, resuming dispatch", "disk", r.Name, "free", r.Free)
	}
}

// watchedPaths is a project's repo path and the Go cache path: the paths whose
// disks the project depends on. A project with no repo path has none.
func (s *Server) watchedPaths(projectDir string) []string {
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil || pf.Repo == nil {
		return nil
	}
	paths := []string{*pf.Repo}
	if gc := s.goCache(); gc != "" {
		paths = append(paths, gc)
	}
	return paths
}

// heldDiskOf returns the first held disk project watches, from the watcher's
// state. It takes no reading.
func (s *Server) heldDiskOf(project string) (DiskStatus, bool) {
	for _, d := range s.diskStatuses(false) {
		if d.Held && slices.Contains(d.Projects, project) {
			return d, true
		}
	}
	return DiskStatus{}, false
}

// freshHeldDisk reads every disk project watches (its repo disk and the Go
// cache disk) and returns the first one that is held. A project with no repo
// path has no disk to read.
func (s *Server) freshHeldDisk(project string) (DiskStatus, bool) {
	dir, err := s.projectDir(project)
	if err != nil {
		return DiskStatus{}, false
	}
	var heldID string
	now := time.Now()
	for _, p := range s.watchedPaths(dir) {
		r, err := s.freeSpace(p)
		if err != nil {
			s.log.Warn("read free space", "path", p, "err", err)
			continue
		}
		if s.observe(r, p, nil, now) && heldID == "" {
			heldID = r.ID
		}
	}
	if heldID == "" {
		return DiskStatus{}, false
	}
	return s.diskStatus(heldID), true
}

// diskStatus is one known disk's status.
func (s *Server) diskStatus(id string) DiskStatus {
	d := &s.disks
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.disks[id].status(id)
}

// diskLowReason says which disk holds dispatch and how much is free on it.
func diskLowReason(d DiskStatus) string {
	return d.Name + " has " + humanize.Bytes(d.FreeBytes) + " free"
}

// diskStatuses lists the known disks by id; listedOnly keeps those whose warn
// mark is set or that are held.
func (s *Server) diskStatuses(listedOnly bool) []DiskStatus {
	d := &s.disks
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []DiskStatus
	for id, st := range d.disks {
		if listedOnly && !st.warned && !st.held {
			continue
		}
		out = append(out, st.status(id))
	}
	slices.SortFunc(out, func(a, b DiskStatus) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// listedDisks lists the disks the snapshot shows: warn mark set or held, by id.
func (s *Server) listedDisks() []DiskStatus { return s.diskStatuses(true) }

// DiskReport is the disk status route's result: the effective thresholds as
// size strings ("off" when a threshold is off) and every known disk.
type DiskReport struct {
	WarnBelow  string       `json:"warn_below"`
	CleanBelow string       `json:"clean_below"`
	Disks      []DiskStatus `json:"disks"`
}

func (s *Server) diskStatusRead(w http.ResponseWriter, _ *http.Request) {
	ds := s.settings.Load().DiskSpace
	writeJSON(w, DiskReport{WarnBelow: sizeOrOff(ds.WarnBelow), CleanBelow: sizeOrOff(ds.CleanBelow), Disks: s.diskStatuses(false)})
}

func sizeOrOff(b config.ByteSize) string {
	if b == 0 {
		return "off"
	}
	return b.String()
}

func (s *Server) freeSpace(path string) (DiskReading, error) {
	if s.cfg.FreeSpace != nil {
		return s.cfg.FreeSpace(path)
	}
	return readFreeSpace(path)
}

// goCache is the build cache path, looked up once: the claim path needs it on every tick.
func (s *Server) goCache() string {
	d := &s.disks
	d.goCacheOnce.Do(func() { d.goCachePath = s.lookupGoCache() })
	return d.goCachePath
}

// lookupGoCache asks go for its build cache; "" means it is not watched.
func (s *Server) lookupGoCache() string {
	lookup := s.cfg.GoCacheDir
	if lookup == nil {
		lookup = func() (string, error) {
			out, err := exec.Command("go", "env", "GOCACHE").Output()
			return strings.TrimSpace(string(out)), err
		}
	}
	path, err := lookup()
	if err != nil || path == "" {
		s.log.Warn("go build cache not found; watching repo disks only", "err", err)
		return ""
	}
	return path
}

// checkDisks reads every project's repo disk and the Go cache disk once.
func (s *Server) checkDisks() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	type watched struct {
		reading  DiskReading
		probe    string
		projects []string
	}
	byDisk := map[string]*watched{}
	add := func(path, project string) {
		r, err := s.freeSpace(path)
		if err != nil {
			s.log.Warn("read free space", "path", path, "err", err)
			return
		}
		w := byDisk[r.ID]
		if w == nil {
			w = &watched{reading: r, probe: path}
			byDisk[r.ID] = w
		}
		if !slices.Contains(w.projects, project) {
			w.projects = append(w.projects, project)
		}
	}
	for _, dir := range dirs {
		name := tickets.ProjectName(dir)
		for _, p := range s.watchedPaths(dir) {
			add(p, name)
		}
	}
	now := time.Now()
	for _, w := range byDisk {
		slices.Sort(w.projects)
		s.observe(w.reading, w.probe, w.projects, now)
	}
}

// keepDisksWatched reads free space on a timer until ctx ends. The serve path
// takes the first reading itself, before the claim loop starts.
func (s *Server) keepDisksWatched(ctx context.Context) {
	interval := s.cfg.DiskPollInterval
	if interval <= 0 {
		interval = defaultDiskPoll
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.checkDisks()
		}
	}
}
