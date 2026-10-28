package server

import (
	"context"
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

// DiskStatus is one warned disk in the snapshot.
type DiskStatus struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	FreeBytes uint64   `json:"free_bytes"`
	Projects  []string `json:"projects"`
}

type diskState struct {
	reading  DiskReading
	at       time.Time
	projects []string
	warned   bool
}

// diskWatch is the per-disk state. Every reading goes through observe, under mu.
type diskWatch struct {
	mu    sync.Mutex
	disks map[string]*diskState
}

// observe applies one reading to the state machine. projects is who watches
// the disk now; nil keeps the last list.
func (s *Server) observe(r DiskReading, projects []string, now time.Time) {
	warn := uint64(s.settings.Load().DiskSpace.WarnBelow)
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
	if projects != nil {
		st.projects = projects
	}
	var changed, set bool
	switch {
	case !st.warned && warn > 0 && r.Free < warn:
		st.warned, changed, set = true, true, true
	case st.warned && (warn == 0 || r.Free > warn+diskWarnMargin):
		st.warned, changed = false, true
	}
	d.mu.Unlock()
	if !changed {
		return
	}
	s.events.publish(EventDiskWarnChanged, "")
	if set {
		s.log.Warn("disk low on space", "disk", r.Name, "free", r.Free, "warn_below", warn)
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeDiskWarn, Emoji: "💾", Title: "low disk space on " + r.Name,
			Detail: humanize.Bytes(r.Free) + " free, under the warn threshold of " + humanize.Bytes(warn)})
	}
}

// warnedDisks lists the disks whose warn mark is set, by id.
func (s *Server) warnedDisks() []DiskStatus {
	d := &s.disks
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []DiskStatus
	for id, st := range d.disks {
		if st.warned {
			out = append(out, DiskStatus{ID: id, Name: st.reading.Name, FreeBytes: st.reading.Free, Projects: slices.Clone(st.projects)})
		}
	}
	slices.SortFunc(out, func(a, b DiskStatus) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func (s *Server) freeSpace(path string) (DiskReading, error) {
	if s.cfg.FreeSpace != nil {
		return s.cfg.FreeSpace(path)
	}
	return readFreeSpace(path)
}

// goCachePath asks go for its build cache; "" means it is not watched.
func (s *Server) goCachePath() string {
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
func (s *Server) checkDisks(goCache string) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	type watched struct {
		reading  DiskReading
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
			w = &watched{reading: r}
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
		s.observe(w.reading, w.projects, now)
	}
}

// keepDisksWatched reads free space at start and then on a timer until ctx ends.
func (s *Server) keepDisksWatched(ctx context.Context) {
	interval := s.cfg.DiskPollInterval
	if interval <= 0 {
		interval = defaultDiskPoll
	}
	goCache := s.goCachePath()
	s.checkDisks(goCache)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.checkDisks(goCache)
		}
	}
}
