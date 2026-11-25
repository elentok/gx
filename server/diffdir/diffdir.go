// Package diffdir answers whether a session's directory can show a diff of its
// project: it shares the project repo's git common dir, so the project's object
// store holds the directory's snapshot objects. A separate clone does not.
package diffdir

import (
	"path/filepath"
	"sync"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets/ticketgit"
)

// CommonDir reads a directory's git common dir; an error means it is no repo.
type CommonDir func(dir string) (string, error)

// Checker caches one common-dir lookup per directory, so a list request reads
// maps and never runs git for a directory it has seen.
type Checker struct {
	store  string
	lookup CommonDir

	mu      sync.Mutex
	commons map[string]*commonEntry
	repos   map[string]string // project -> repo root
}

// New builds a Checker over the ticket store. A nil lookup runs git.
func New(store string, lookup CommonDir) *Checker {
	if lookup == nil {
		lookup = func(dir string) (string, error) {
			_, common, err := git.GitDirs(dir)
			return common, err
		}
	}
	return &Checker{store: store, lookup: lookup, commons: map[string]*commonEntry{}, repos: map[string]string{}}
}

// Qualifies reports whether dir is in project's repo or one of its linked worktrees.
func (c *Checker) Qualifies(project, dir string) bool {
	if dir == "" {
		return false
	}
	root := c.repoRoot(project)
	if root == "" {
		return false
	}
	want := c.commonDir(root)
	return want != "" && c.commonDir(dir) == want
}

// Forget drops dir's cached lookup, for a session that ended.
func (c *Checker) Forget(dir string) {
	c.mu.Lock()
	delete(c.commons, filepath.Clean(dir))
	c.mu.Unlock()
}

func (c *Checker) repoRoot(project string) string {
	c.mu.Lock()
	root, ok := c.repos[project]
	c.mu.Unlock()
	if ok {
		return root
	}
	root = ticketgit.ProjectRepoRoot(c.store, project)
	if root != "" {
		c.mu.Lock()
		c.repos[project] = root
		c.mu.Unlock()
	}
	return root
}

func (c *Checker) commonDir(dir string) string {
	dir = filepath.Clean(dir)
	c.mu.Lock()
	e, ok := c.commons[dir]
	if !ok {
		e = &commonEntry{}
		c.commons[dir] = e
	}
	c.mu.Unlock()
	// Concurrent first reads of one directory share a single lookup.
	e.once.Do(func() {
		common, err := c.lookup(dir)
		if err == nil {
			e.common = common
		}
	})
	return e.common
}

// commonEntry is one directory's lookup; common is "" when it is no repo.
type commonEntry struct {
	once   sync.Once
	common string
}
