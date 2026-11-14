// Package files decides whether gx may host a path as a Hosted file. It has
// no route, header or token code; every file feature calls Check.
package files

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Hosted is a file gx may serve.
type Hosted struct {
	Path    string // symlink-resolved canonical path
	Size    int64
	ModTime time.Time
}

// Checker answers "may gx host this absolute path?". Source funcs are called
// on every Check so new worktrees and a moved tickets store need no restart.
type Checker struct {
	// BuiltinRoots lists project repos, their worktrees and the tickets store.
	BuiltinRoots func() []string
	// ExtraRoots lists the configured extra roots.
	ExtraRoots func() []string
	// TempRoots lists the shared temp folders.
	TempRoots func() []string
	// DeniedDirs are gx's own folders (state, config, sessions, ...), denied whole.
	DeniedDirs func() []string
	// DataDir is denied except TicketsStore.
	DataDir func() string
	// TicketsStore is the tickets store root.
	TicketsStore func() string
	// UID is the gx user id that must own temp files and their folders.
	UID uint32

	// statOverride lets tests simulate a foreign owner on a single folder.
	statOverride func(path string, info os.FileInfo) (uid uint32, nlink uint64, ok bool)
}

// DefaultTempRoots returns /tmp, /private/tmp and the server's $TMPDIR.
func DefaultTempRoots() []string {
	roots := []string{"/tmp", "/private/tmp"}
	if t := os.Getenv("TMPDIR"); t != "" {
		roots = append(roots, t)
	}
	return roots
}

// Check returns the hosted file for an absolute, already joined path. The
// bool is the only verdict: it never says which rule failed.
func (c *Checker) Check(path string) (Hosted, bool) {
	if !filepath.IsAbs(path) {
		return Hosted{}, false
	}
	requested := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(requested)
	if err != nil {
		return Hosted{}, false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return Hosted{}, false
	}
	roots := resolveAll(call(c.BuiltinRoots), call(c.ExtraRoots), call(c.TempRoots))
	if !insideAny(resolved, roots) {
		return Hosted{}, false
	}
	if tempRoot, ok := containing(resolved, resolveAll(call(c.TempRoots))); ok && !c.ownedTemp(resolved, tempRoot, info) {
		return Hosted{}, false
	}
	if c.inGxFolder(resolved) {
		return Hosted{}, false
	}
	if denied(requested) || denied(resolved) {
		return Hosted{}, false
	}
	return Hosted{Path: resolved, Size: info.Size(), ModTime: info.ModTime()}, true
}

func (c *Checker) ownedTemp(resolved, tempRoot string, info os.FileInfo) bool {
	uid, nlink, ok := c.stat(resolved, info)
	if !ok || uid != c.UID || nlink != 1 {
		return false
	}
	for dir := filepath.Dir(resolved); dir != tempRoot && strings.HasPrefix(dir, tempRoot); dir = filepath.Dir(dir) {
		di, err := os.Stat(dir)
		if err != nil {
			return false
		}
		duid, _, ok := c.stat(dir, di)
		if !ok || duid != c.UID {
			return false
		}
	}
	return true
}

func (c *Checker) stat(path string, info os.FileInfo) (uid uint32, nlink uint64, ok bool) {
	if c.statOverride != nil {
		return c.statOverride(path, info)
	}
	return sysStat(info)
}

func sysStat(info os.FileInfo) (uid uint32, nlink uint64, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return st.Uid, uint64(st.Nlink), true
}

func (c *Checker) inGxFolder(resolved string) bool {
	if insideAny(resolved, resolveAll(call(c.DeniedDirs))) {
		return true
	}
	if c.DataDir == nil {
		return false
	}
	data := resolveAll([]string{c.DataDir()})
	if !insideAny(resolved, data) {
		return false
	}
	var store []string
	if c.TicketsStore != nil {
		store = resolveAll([]string{c.TicketsStore()})
	}
	return !insideAny(resolved, store)
}

func call(f func() []string) []string {
	if f == nil {
		return nil
	}
	return f()
}

// resolveAll resolves symlinks in every non-empty path (macOS /tmp and
// /var/folders are symlinks). A path that does not exist keeps its cleaned form.
func resolveAll(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		for _, p := range l {
			if p == "" {
				continue
			}
			p = filepath.Clean(p)
			if r, err := filepath.EvalSymlinks(p); err == nil {
				p = r
			}
			out = append(out, p)
		}
	}
	return out
}

func containing(path string, roots []string) (string, bool) {
	for _, r := range roots {
		if inside(path, r) {
			return r, true
		}
	}
	return "", false
}

func insideAny(path string, roots []string) bool {
	_, ok := containing(path, roots)
	return ok
}

func inside(path, root string) bool {
	if root == string(filepath.Separator) {
		return true
	}
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}
