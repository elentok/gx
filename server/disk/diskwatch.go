// Package disk watches free space on the disks the projects live on, holds
// dispatch when one runs low, and cleans it.
package disk

import (
	"context"
	"log/slog"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Config is the slice of the server's config the disk watcher reads. A zero
// field takes the default its comment names.
type Config struct {
	// FreeSpace reads the disk holding a path; nil reads the real one.
	FreeSpace func(path string) (wire.DiskReading, error)
	// GoCacheDir finds the Go build cache; nil runs `go env GOCACHE`.
	GoCacheDir func() (string, error)
	// DiskPollInterval is how often the watcher reads; zero means one minute.
	DiskPollInterval time.Duration
	// CleanupRunner runs one cleanup command in dir; nil runs the real command.
	CleanupRunner func(ctx context.Context, argv []string, dir string) ([]byte, error)
	// CleanupTimeout is the time limit per cleanup command; zero means ten minutes.
	CleanupTimeout time.Duration
	// TempSweepDir is the temp dir a clean sweeps; empty covers os.TempDir() and /tmp.
	TempSweepDir string
	// TempSweepMinAge is how long a temp dir sits untouched before the sweep removes it.
	TempSweepMinAge time.Duration
	// CacheGateCap bounds the wait for running Go builds before a clean.
	CacheGateCap time.Duration
	// CacheGateScan replaces the `go` process scan; nil scans the real processes.
	CacheGateScan func() (bool, error)
	StateDir      string
	TicketStore   string
	Env           []string
}

// Deps is what the watcher needs from the root. HealthSink, when set, is called
// after every reading so the health meter follows the disk state; disk does not
// import health.
type Deps struct {
	Config     Config
	Settings   *settings.Holder
	Projects   *projects.Registry
	Events     *broker.Broker
	Chat       *notify.ServerChat
	Guards     *runs.Guards
	Log        *slog.Logger
	HealthSink func(now time.Time)
}

// Disk is the per-disk state, its watch loop and its clean flow.
type Disk struct {
	cfg        Config
	settings   *settings.Holder
	projects   *projects.Registry
	events     *broker.Broker
	chat       *notify.ServerChat
	guards     *runs.Guards
	log        *slog.Logger
	healthSink func(now time.Time)
	disks      diskWatch
}

// New builds the disk watcher.
func New(d Deps) *Disk {
	return &Disk{cfg: d.Config, settings: d.Settings, projects: d.Projects, events: d.Events, chat: d.Chat,
		guards: d.Guards, log: d.Log, healthSink: d.HealthSink}
}

const (
	defaultDiskPoll = time.Minute
	// diskWarnMargin is how far above the warn threshold a disk must climb
	// before its mark clears, so a reading near the threshold does not flap.
	diskWarnMargin = 2 * humanize.GByte
)

type diskState struct {
	reading   wire.DiskReading
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

func (st *diskState) status(id string) wire.DiskStatus {
	return wire.DiskStatus{ID: id, Name: st.reading.Name, FreeBytes: st.reading.Free,
		TotalBytes: st.reading.Total, Projects: slices.Clone(st.projects), Warned: st.warned, Held: st.held, ReadAt: st.at,
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
// run after the lock is released. warn and clean are the thresholds the
// reading was judged against. changed is only set by readTransition.
type diskChange struct {
	changed, warnSet, holdSet, resumed, startClean bool
	warn, clean                                    uint64
}

// diskSnapshot is the disk's state after a reading, copied under the lock.
type diskSnapshot struct {
	held      bool
	projects  []string
	lastClean string
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
	c := diskChange{warn: warn, clean: clean}
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
	c := diskChange{warn: warn, clean: clean}
	c.warnSet, _ = st.updateWarn(free, warn)
	st.cleaning = false
	switch {
	case end.failed != "":
		st.lastClean = "failed: " + end.failed
	case end.noWork:
		st.lastClean = wire.NoCleanCommand
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
func (s *Disk) transition(r wire.DiskReading, probe string, projects []string, now time.Time,
	fn func(st *diskState, warn, clean uint64) diskChange) (diskChange, diskSnapshot) {
	ds := s.settings.Load().DiskSpace
	d := &s.disks
	if s.healthSink != nil {
		defer s.healthSink(now) // runs after the unlock below
	}
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
	return c, diskSnapshot{held: st.held, projects: slices.Clone(st.projects), lastClean: st.lastClean}
}

// observe applies one ordinary reading to the state machine and reports
// whether the disk is held after it.
func (s *Disk) observe(r wire.DiskReading, probe string, projects []string, now time.Time) (held bool) {
	c, snap := s.transition(r, probe, projects, now, func(st *diskState, warn, clean uint64) diskChange {
		return st.readTransition(r.Free, warn, clean)
	})
	if !c.changed {
		return snap.held
	}
	s.publishChange(r, c)
	if c.resumed {
		s.chat.Notice(notify.ServerNotice{Kind: wire.NoticeDiskResume, Emoji: "💾", Title: "dispatch resumed on " + r.Name,
			Detail: humanize.Bytes(r.Free) + " free; queued tickets start again"})
		s.guards.Kick()
	}
	if c.startClean {
		s.startClean(r.ID, probe, snap.projects)
	}
	return snap.held
}

// observeCleanEnd applies the reading that ends a clean and reports whether
// the disk is held after it.
func (s *Disk) observeCleanEnd(r wire.DiskReading, probe string, end cleanEnd) (held bool) {
	c, snap := s.transition(r, probe, nil, time.Now(), func(st *diskState, warn, clean uint64) diskChange {
		return st.endTransition(r.Free, warn, clean, end)
	})
	s.publishChange(r, c)
	if c.resumed {
		s.guards.Kick()
	}
	s.sendCleanNotice(r.Name, snap.lastClean, r.Free, snap.held)
	return snap.held
}

// publishChange announces a changed disk: the event, the warn and hold logs and
// the warn notice.
func (s *Disk) publishChange(r wire.DiskReading, c diskChange) {
	s.events.Publish(wire.EventDiskWarnChanged, "")
	if c.warnSet {
		s.log.Warn("disk low on space", "disk", r.Name, "free", r.Free, "warn_below", c.warn)
		s.chat.Notice(notify.ServerNotice{Kind: wire.NoticeDiskWarn, Emoji: "💾", Title: "low disk space on " + r.Name,
			Detail: humanize.Bytes(r.Free) + " free, under the warn threshold of " + humanize.Bytes(c.warn), HoldsWork: true})
	}
	if c.holdSet {
		s.log.Warn("disk low, holding dispatch", "disk", r.Name, "free", r.Free, "clean_below", c.clean)
	}
	if c.resumed {
		s.log.Info("disk recovered, resuming dispatch", "disk", r.Name, "free", r.Free)
	}
}

// watchedPaths is a project's repo path and the Go cache path: the paths whose
// disks the project depends on. A project with no repo path has none.
func (s *Disk) watchedPaths(projectDir string) []string {
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

// HeldDiskOf returns the first held disk project watches, from the watcher's
// state. It takes no reading.
func (s *Disk) HeldDiskOf(project string) (wire.DiskStatus, bool) {
	for _, d := range s.Statuses(false) {
		if d.Held && slices.Contains(d.Projects, project) {
			return d, true
		}
	}
	return wire.DiskStatus{}, false
}

// FreshHeldDisk reads every disk project watches (its repo disk and the Go
// cache disk) and returns the first one that is held. A project with no repo
// path has no disk to read.
func (s *Disk) FreshHeldDisk(project string) (wire.DiskStatus, bool) {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return wire.DiskStatus{}, false
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
		return wire.DiskStatus{}, false
	}
	return s.diskStatus(heldID), true
}

// diskStatus is one known disk's status.
func (s *Disk) diskStatus(id string) wire.DiskStatus {
	d := &s.disks
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.disks[id].status(id)
}

// LowReason says which disk holds dispatch and how much is free on it.
func LowReason(d wire.DiskStatus) string {
	return d.Name + " has " + humanize.Bytes(d.FreeBytes) + " free"
}

// Statuses lists the known disks by id; listedOnly keeps those whose warn
// mark is set or that are held.
func (s *Disk) Statuses(listedOnly bool) []wire.DiskStatus {
	d := &s.disks
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []wire.DiskStatus
	for id, st := range d.disks {
		if listedOnly && !st.warned && !st.held {
			continue
		}
		out = append(out, st.status(id))
	}
	slices.SortFunc(out, func(a, b wire.DiskStatus) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Listed lists the disks the snapshot shows: warn mark set or held, by id.
func (s *Disk) Listed() []wire.DiskStatus { return s.Statuses(true) }

func (s *Disk) statusRead(w http.ResponseWriter, _ *http.Request) {
	ds := s.settings.Load().DiskSpace
	wire.WriteJSON(w, wire.DiskReport{WarnBelow: SizeOrOff(ds.WarnBelow), CleanBelow: SizeOrOff(ds.CleanBelow), Disks: s.Statuses(false)})
}

// SizeOrOff prints a threshold, "off" for zero.
func SizeOrOff(b config.ByteSize) string {
	if b == 0 {
		return "off"
	}
	return b.String()
}

func (s *Disk) freeSpace(path string) (wire.DiskReading, error) {
	if s.cfg.FreeSpace != nil {
		return s.cfg.FreeSpace(path)
	}
	return readFreeSpace(path)
}

// goCache is the build cache path, looked up once: the claim path needs it on every tick.
func (s *Disk) goCache() string {
	d := &s.disks
	d.goCacheOnce.Do(func() { d.goCachePath = s.lookupGoCache() })
	return d.goCachePath
}

// lookupGoCache asks go for its build cache; "" means it is not watched.
func (s *Disk) lookupGoCache() string {
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

// Check reads every project's repo disk and the Go cache disk once.
func (s *Disk) Check() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	type watched struct {
		reading  wire.DiskReading
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

// Run reads free space on a timer until ctx ends. The serve path
// takes the first reading itself (Check), before the claim loop starts.
func (s *Disk) Run(ctx context.Context) {
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
			s.Check()
		}
	}
}
