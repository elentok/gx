package nativerunner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// ErrUnknownRunner is an agent directory whose meta.json names a runner with
// no layout. Such a directory is never pruned or cleaned up.
var ErrUnknownRunner = errors.New("agent directory has an unknown runner")

// layout is what one runner keeps in its agent directories.
type layout struct {
	// logs stay for inspection until Prune. The first is the primary log
	// whose absence marks the directory pruned.
	logs []string
	// transient files are removed by Cleanup on land or cancel.
	transient []string
	alive     func(procs ProcessTable, meta Meta) (bool, error)
}

var layouts = map[string]layout{
	runnerName: {logs: []string{OutFile, ErrFile}, transient: []string{StdinFile}, alive: headlessAlive},
}

func layoutOf(meta Meta) (layout, error) {
	l, ok := layouts[meta.Runner]
	if !ok {
		return layout{}, fmt.Errorf("%w %q: %s", ErrUnknownRunner, meta.Runner, meta.Label)
	}
	return l, nil
}

// readLayout reads dir's meta.json and the layout of the runner it names.
func readLayout(dir string) (Meta, layout, error) {
	meta, err := readMeta(dir)
	if err != nil {
		return Meta{}, layout{}, err
	}
	l, err := layoutOf(meta)
	return meta, l, err
}

// Cleanup removes label's transient files once its iteration landed or was
// cancelled. Logs and meta.json stay for inspection until Prune.
func (h *Headless) Cleanup(label string) error {
	dir := filepath.Join(h.Root, label)
	_, l, err := readLayout(dir)
	if err != nil {
		// Nothing launched here, or not ours to touch.
		return nil
	}
	return removeAll(dir, l.transient)
}

// Dir is an agent directory read through the layout of the runner its
// meta.json names, so readers like watch need no runner of their own.
type Dir struct {
	Path string
	Meta Meta
	l    layout
}

// OpenDir reads dir's meta.json. A missing directory is
// agentrunner.ErrNotFound; an unknown runner is ErrUnknownRunner.
func OpenDir(dir string) (Dir, error) {
	meta, l, err := readLayout(dir)
	if err != nil {
		return Dir{}, err
	}
	return Dir{Path: dir, Meta: meta, l: l}, nil
}

// Log is the primary log's path. It is gone once Pruned.
func (d Dir) Log() string { return filepath.Join(d.Path, d.l.logs[0]) }

// Pruned reports whether Prune already removed the logs.
func (d Dir) Pruned() bool { return d.l.pruned(d.Path) }

// Alive reports whether the agent still runs. Nil procs means ps.
func (d Dir) Alive(procs ProcessTable) (bool, error) { return d.l.alive(orPS(procs), d.Meta) }

// Inspect reads label's meta.json and reports whether its agent still runs.
// Errors are OpenDir's.
func (h *Headless) Inspect(label string) (Meta, bool, error) {
	d, err := OpenDir(filepath.Join(h.Root, label))
	if err != nil {
		return Meta{}, false, err
	}
	live, err := d.Alive(h.procs())
	return d.Meta, live, err
}

// Prune removes the logs of agent directories untouched for longer than
// maxAge and returns their labels. meta.json stays, so watch can still name
// the claude transcript. It keeps any directory whose agent still runs and
// any label parked reports true for, so a parked ticket's logs stay.
func (h *Headless) Prune(maxAge time.Duration, now time.Time, parked func(label string) bool) ([]string, error) {
	entries, err := os.ReadDir(h.Root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var pruned []string
	for _, e := range entries {
		label := e.Name()
		if !e.IsDir() || parked(label) {
			continue
		}
		dir := filepath.Join(h.Root, label)
		meta, l, err := readLayout(dir)
		if err != nil {
			// Not an agent directory, one too broken to judge, or another
			// runner's; leave it.
			continue
		}
		if l.pruned(dir) || !l.lastTouched(dir).Before(now.Add(-maxAge)) {
			continue
		}
		h.mu.Lock()
		_, adopted := h.sessions[label]
		h.mu.Unlock()
		if adopted {
			continue
		}
		if live, err := l.alive(h.procs(), meta); err != nil || live {
			continue
		}
		if err := removeAll(dir, slices.Concat(l.logs, l.transient)); err != nil {
			return pruned, err
		}
		pruned = append(pruned, label)
	}
	return pruned, nil
}

// AgentsRoot is where a project's native agent directories live.
func AgentsRoot(stateDir, project string) string {
	return filepath.Join(stateDir, "agents", project)
}

func (l layout) pruned(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, l.logs[0]))
	return errors.Is(err, os.ErrNotExist)
}

func (l layout) lastTouched(dir string) time.Time {
	var latest time.Time
	for _, name := range append([]string{MetaFile}, l.logs...) {
		if fi, err := os.Stat(filepath.Join(dir, name)); err == nil && fi.ModTime().After(latest) {
			latest = fi.ModTime()
		}
	}
	return latest
}

func removeAll(dir string, names []string) error {
	for _, name := range names {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
