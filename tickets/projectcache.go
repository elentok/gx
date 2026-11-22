package tickets

import (
	"path/filepath"
	"sync"

	"github.com/elentok/gx/config"
)

// ProjectCache holds a store's project dirs, names and project files for one
// store generation. The generation counter moves on every store rescan, so
// between rescans a page refresh reads the cache and not every project.json.
// A nil *ProjectCache reads the disk each time.
type ProjectCache struct {
	// Read is the project.json read; tests swap it to count reads. Nil means
	// config.ReadProjectFile.
	Read func(dir string) (config.ProjectFile, error)

	gen func() uint64

	mu    sync.Mutex
	at    uint64
	store string
	built bool
	dirs  []string
	files map[string]projectEntry
}

type projectEntry struct {
	pf  config.ProjectFile
	err error
}

// NewProjectCache builds a cache that drops its snapshot whenever gen changes.
func NewProjectCache(gen func() uint64) *ProjectCache { return &ProjectCache{gen: gen} }

func (c *ProjectCache) read(dir string) (config.ProjectFile, error) {
	if c.Read != nil {
		return c.Read(dir)
	}
	return config.ReadProjectFile(dir)
}

// snapshot returns the current generation's files, building them on a miss.
// Callers hold c.mu.
func (c *ProjectCache) snapshot(storePath string) error {
	g := c.gen()
	if c.built && c.at == g && c.store == storePath {
		return nil
	}
	c.built, c.at, c.store, c.dirs, c.files = false, g, storePath, nil, map[string]projectEntry{}
	err := forEachProjectRead(storePath, c.read, func(dir string, pf config.ProjectFile) bool {
		c.dirs = append(c.dirs, dir)
		c.files[dir] = projectEntry{pf: pf}
		return false
	})
	if err != nil {
		return err
	}
	c.built = true
	return nil
}

// Dirs is ProjectDirs for the cached generation.
func (c *ProjectCache) Dirs(storePath string) ([]string, error) {
	if c == nil {
		return ProjectDirs(storePath)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.snapshot(storePath); err != nil {
		return nil, err
	}
	return append([]string(nil), c.dirs...), nil
}

// File is config.ReadProjectFile for the cached generation. Dirs builds the
// snapshot; a dir it does not hold, or a read before any Dirs call, goes to
// disk and is not cached.
func (c *ProjectCache) File(dir string) (config.ProjectFile, error) {
	if c == nil {
		return config.ReadProjectFile(dir)
	}
	c.mu.Lock()
	if c.built && c.at == c.gen() {
		if e, ok := c.files[dir]; ok {
			c.mu.Unlock()
			return e.pf, e.err
		}
	}
	c.mu.Unlock()
	return c.read(dir)
}

// Name is ProjectName for the cached generation.
func (c *ProjectCache) Name(dir string) string {
	if pf, err := c.File(dir); err == nil && pf.Name != nil && *pf.Name != "" {
		return *pf.Name
	}
	return filepath.Base(dir)
}
