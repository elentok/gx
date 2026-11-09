package settingsapi

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/fsutil"
)

// Target names the file a write goes to. Project is empty for the global
// config file.
type Target struct {
	Project string
}

// Hooks lets one key run code around a write. Before may refuse by returning
// an error (a *Refusal for a user-facing reason); After may add a notice.
type Hooks interface {
	Before(t Target, changes []Change) error
	After(t Target, changes []Change) (notice string)
}

// Result is what a successful write saved. Values holds each changed key's
// saved value, or nil for a removed key.
type Result struct {
	Values  map[string][]byte
	Notices []string
}

// Writer owns the settings files. One lock covers each whole
// read-modify-write, so concurrent saves both land.
type Writer struct {
	Registry   *registry.Registry
	GlobalPath string
	ProjectDir func(project string) (string, error)
	Hooks      Hooks
	mu         sync.Mutex
}

// Write applies changes to the target file, re-reading it under the lock so
// edits made since the page loaded survive (last write wins per key).
func (w *Writer) Write(t Target, changes []Change) (Result, error) {
	scope, path, err := w.resolve(t)
	if err != nil {
		return Result{}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	doc, err := os.ReadFile(path)
	missing := errors.Is(err, fs.ErrNotExist)
	switch {
	case missing:
		doc = []byte("{}")
	case err != nil:
		return Result{}, err
	}

	if w.Hooks != nil {
		if err := w.Hooks.Before(t, changes); err != nil {
			return Result{}, err
		}
	}
	out, err := Apply(w.Registry, scope, doc, changes)
	if err != nil {
		return Result{}, err
	}

	if missing && allRemoves(changes) {
		return Result{Values: savedValues(changes)}, nil
	}
	if missing {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Result{}, err
		}
	}
	if err := fsutil.WriteFileAtomic(path, out); err != nil {
		return Result{}, err
	}

	res := Result{Values: savedValues(changes)}
	if w.Hooks != nil {
		if n := w.Hooks.After(t, changes); n != "" {
			res.Notices = append(res.Notices, n)
		}
	}
	return res, nil
}

func (w *Writer) resolve(t Target) (Scope, string, error) {
	if t.Project == "" {
		return ScopeGlobal, w.GlobalPath, nil
	}
	dir, err := w.ProjectDir(t.Project)
	if err != nil {
		return 0, "", err
	}
	return ScopeProject, filepath.Join(dir, config.ProjectFileName), nil
}

func allRemoves(changes []Change) bool {
	for _, c := range changes {
		if !c.Remove {
			return false
		}
	}
	return true
}

func savedValues(changes []Change) map[string][]byte {
	m := make(map[string][]byte, len(changes))
	for _, c := range changes {
		if c.Remove {
			m[c.Key.String()] = nil
		} else {
			m[c.Key.String()] = c.Value
		}
	}
	return m
}
