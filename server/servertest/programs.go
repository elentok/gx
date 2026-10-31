package servertest

import (
	"errors"
	"sync"
)

// Programs is the server's PATH lookup in a test: every program is found
// until Missing marks it gone.
type Programs struct {
	mu      sync.Mutex
	missing map[string]bool
}

// Missing takes the named programs off PATH.
func (p *Programs) Missing(names ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.missing == nil {
		p.missing = map[string]bool{}
	}
	for _, n := range names {
		p.missing[n] = true
	}
}

// Install puts the named programs back on PATH.
func (p *Programs) Install(names ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, n := range names {
		delete(p.missing, n)
	}
}

// LookPath is the server's Config.LookPath.
func (p *Programs) LookPath(file string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.missing[file] {
		return "", errors.New(file + ": not found")
	}
	return "/fake/bin/" + file, nil
}
