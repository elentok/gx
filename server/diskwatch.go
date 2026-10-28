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

// observe applies one reading to the state machine. projects is who watches
// the disk now; nil keeps the last list. probe is a path on the disk, read
// again when a clean ends. end is set only by the end of a clean. It reports
// whether the disk is held after this reading.
func (s *Server) observe(r DiskReading, probe string, projects []string, now time.Time, end *cleanEnd) (held bool) {
	ds := s.settings.Load().DiskSpace
	warn, clean := uint64(ds.WarnBelow), uint64(ds.CleanBelow)
	d := &s.disks
	d.mu.Lock()
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
	var changed, warnSet, holdSet, resumed, startClean bool
	switch {
	case !st.warned && warn > 0 && r.Free < warn:
		st.warned, changed, warnSet = true, true, true
	case st.warned && (warn == 0 || r.Free > warn+diskWarnMargin):
		st.warned, changed = false, true
	}
	// The end-of-clean reading never re-arms; a later one does.
	if end == nil && st.spent && r.Free > max(warn, clean+diskWarnMargin) {
		st.spent = false
	}
	low := clean > 0 && r.Free < clean
	switch {
	case end != nil:
		// One rule ends every clean: the fresh reading decides the hold.
		st.cleaning = false
		st.lastClean = "freed " + humanize.Bytes(max(r.Free, st.cleanFree)-st.cleanFree)
		if end.failed != "" {
			st.lastClean = "failed: " + end.failed
		}
		changed = true
		if st.held != low {
			st.held, resumed = low, !low
		}
	case !st.held && low:
		st.held, changed, holdSet = true, true, true
	case st.held && !low && !st.cleaning:
		st.held, changed, resumed = false, true, true
	}
	if end == nil && low && !st.spent && !st.cleaning {
		st.spent, st.cleaning, st.cleanFree = true, true, r.Free
		changed, startClean = true, true
	}
	held = st.held
	disk, projectList, lastClean := r.Name, slices.Clone(st.projects), st.lastClean
	d.mu.Unlock()
	if !changed {
		return held
	}
	s.events.publish(EventDiskWarnChanged, "")
	if warnSet {
		s.log.Warn("disk low on space", "disk", r.Name, "free", r.Free, "warn_below", warn)
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeDiskWarn, Emoji: "💾", Title: "low disk space on " + r.Name,
			Detail: humanize.Bytes(r.Free) + " free, under the warn threshold of " + humanize.Bytes(warn)})
	}
	if holdSet {
		s.log.Warn("disk low, holding dispatch", "disk", r.Name, "free", r.Free, "clean_below", clean)
	}
	if resumed {
		s.log.Info("disk recovered, resuming dispatch", "disk", r.Name, "free", r.Free)
		if end == nil {
			s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeDiskResume, Emoji: "💾", Title: "dispatch resumed on " + r.Name,
				Detail: humanize.Bytes(r.Free) + " free; queued tickets start again"})
		}
		s.kickRunner()
	}
	if end != nil {
		s.sendCleanNotice(disk, lastClean, r.Free, held)
	}
	if startClean {
		s.startClean(r.ID, probe, projectList)
	}
	return held
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
	pf, err := config.ReadProjectFile(dir)
	if err != nil || pf.Repo == nil {
		return DiskStatus{}, false
	}
	paths := []string{*pf.Repo}
	if gc := s.goCache(); gc != "" {
		paths = append(paths, gc)
	}
	var held *DiskStatus
	now := time.Now()
	for _, p := range paths {
		r, err := s.freeSpace(p)
		if err != nil {
			s.log.Warn("read free space", "path", p, "err", err)
			continue
		}
		if s.observe(r, p, nil, now, nil) && held == nil {
			d := DiskStatus{ID: r.ID, Name: r.Name, FreeBytes: r.Free}
			held = &d
		}
	}
	if held == nil {
		return DiskStatus{}, false
	}
	return *held, true
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
	goCache := s.goCache()
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
		pf, err := config.ReadProjectFile(dir)
		if err != nil || pf.Repo == nil {
			continue
		}
		name := tickets.ProjectName(dir)
		add(*pf.Repo, name)
		if goCache != "" {
			add(goCache, name)
		}
	}
	now := time.Now()
	for _, w := range byDisk {
		slices.Sort(w.projects)
		s.observe(w.reading, w.probe, w.projects, now, nil)
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
